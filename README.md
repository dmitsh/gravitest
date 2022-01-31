## Design proposal: process manager API

This document outlines a design proposal for a prototype of a service API to manage Linux processes.

There are four major components described below:
 - Server
 - Library
 - API
 - Client

### Server

The service is implemented as a server, allowing multiple concurrent client connections.

The server provides the following functionality:
 - exposes gRPC endpoint.
 - leverages mTLS for client-server authentication.
 - implements API proto spec and invokes corresponding calls in the core library.

#### Authentication and authorization

The authentication is implemented via mTLS. The server and the clients are using certificates signed by a common CA.

Each client has an individual certificate with a unique Common Name (`CN`).

If the server cannot authenticate the client, the request will be rejected with an error.

The commands below could be used to generate certificates.


```bash
# create CA key pair and self signed certificate
openssl req -x509 -newkey rsa:4096 -keyout ca.key -out ca.crt -days 365 -nodes -subj "/CN=RootCA" -config host.conf

# create key pair and CSR for server and clients
openssl genrsa -out server.key 2048
openssl genrsa -out client1.key 2048
openssl genrsa -out client2.key 2048

openssl req -new -key server.key -subj "/CN=server" -out server.csr
openssl req -new -key client1.key -subj "/CN=client1" -out client1.csr
openssl req -new -key client2.key -subj "/CN=client2" -out client2.csr

# sign CSR with CA
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -extfile host-server.conf
openssl x509 -req -in client1.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client1.crt -days 365
openssl x509 -req -in client2.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client2.crt -days 365
```

The authorization is implemented by maintaining a key/value user table, where the key is the client ID (extracted from the client certificate), and the value is the bitmap of permitted API calls:
```go
const (
	PermStart  = 0x01
	PermStop   = 0x02
	PermStatus = 0x04
	PermStream = 0x08
)
```

### Library

The library is the core of the server, which does the following:
 - implements process management APIs:
   - `StartProcess` starts a new process.
   - `GetProcessStatus` returns process status.
   - `StreamOutput` returns combined process standard and error output stream.
   - `StopProcess` stops the process.
 - verifies user authorization:
   - the library maintains an authorization table of clients and corresponding bitmap of permitted APIs.
   - at the beginning of each API call the library checks that the client is authorized to call the API.
   - for `GetProcessStatus`, `StreamOutput`, and `StopProcess` APIs, the library verifies that the process has been created by the same client.
   - *note:* the library does not have the notion of the `superuser`, but it can be easily added.
 - implements resource control for the processes.
 - generates UUID for processes (`github.com/google/uuid`).
 - maintains a process table, for both active and terminated processes. The key is process UUID. The values is a structure representing the process:
 ```go
 type Process struct {
	clientID string        // client ID retrieved from the client certificate
	cmd      *exec.Cmd     // process object in Go
	output   bytes.Buffer  // buffer containing combined process standard and error outputs
	status   *proto.Status // process status
}
 ```

The library exposes singleton process manager object holding all aforementioned structures:
```go
type ProcManager struct {
	// process table [process UUID : Process]
	procs     map[string]*Process
	procMutex sync.Mutex

	// permission table [client ID : permission bitmap]
	perm map[string]int

	// process UUID
	uuid string
}
```
#### Resource control

The control over compute, disk I/O and memory resources is provided by means of cgroups.
When the library receives an API call to run a user command, it starts a utility program that:
- clones itself with `syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWPID}`
- creates the following files:
  - `/sys/fs/cgroup/cpu/worker-<UUID>/cpu.shares`. This file contains value that limit number of CPU share for the process.
  - `/sys/fs/cgroup/memory/worker-<UUID>/memory.limit_in_bytes`. This file contains value that limits the amount of the virtual and physical memory for the process.
  - `/sys/fs/cgroup/blkio/worker-<UUID>/blkio.throttle.read_bps_device`. This file contains list of block devices followed by a value that limits the read bandwidth rate for the process.
  - `/sys/fs/cgroup/blkio/worker-<UUID>/blkio.throttle.write_bps_device`. This file contains list of block devices followed by a value that limits the write bandwidth rate for the process.
- creates the following files, containing its own PID (`os.Getpid()`):
  - `/sys/fs/cgroup/cpu/worker-<UUID>/cgroup.procs`
  - `/sys/fs/cgroup/memory/worker-<UUID>/cgroup.procs`
  - `/sys/fs/cgroup/blkio/worker-<UUID>/cgroup.procs`
- executes original user command

### API implementation

The API proto spec is declared in [proto/worker.proto](./proto/worker.proto)

`StartProcess`:
 - Input: executable name and optional list of arguments.
 - Output: process UUID.
 - Action:
   1. verify client authorization to call this API.
   2. generate a new process UUID and create a `Process` object.
   3. set process standard and error output streams to the output buffer.
   4. add a new entry in the process table.
   5. start the process by calling `process.cmd.Start()`, and set status to `running`.
   6. upon process termination, check for errors and update process status accordingly.

   *note:* steps 5 and 6 are executed asynchronously in a go-routine.

`StopProcess`:
 - Input: process UUID.
 - Output: none.
 - Action:
   1. verify client authorization to call this API.
   2. if the process is not found in the process table, return `process not found` error.
   3. if the process is running, terminate it by calling `process.cmd.Process.Kill()`

`GetProcessStatus`:
 - Input: process UUID.
 - Output: process status.
 - Action: if the process is in the process table, return process status from the `Process` object. Otherwise return `process not found` error.

`stream-output`:
 - Input: process UUID.
 - Output: stream of the combined process standard and error outputs.
 - Action:
   1. verify client authorization to call this API.
   2. get output buffer from the `Process` object.
   3. start streaming the buffer (gRPC server-side streaming) until process is running or user interrupted the API call.

### Client

The client is a console application performing the following steps:
 - initiates gRPC connection over mTLS with the server. The TLS certificate could be passed in the command line or specified via environment variable.
 - provides CLI to invoke API calls with the server.
 - produces necessary output.

CLI usage examples:
```bash
$ export CLIENT_CERT="$HOME/.certs/userA.crt"
$ export CLIENT_KEY="$HOME/.certs/userA.key"
$ export CA_CERT="$HOME/.certs/ca.crt"

$ ./client start ping 8.8.8.8
Process UUID: 58e1f565-b1d0-436d-8c25-f453408c2514

./client status 58e1f565-b1d0-436d-8c25-f453408c2514
Process status: StatusRunning

$ ./client stream 58e1f565-b1d0-436d-8c25-f453408c2514
PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=117 time=13.771 ms
64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=20.278 ms
64 bytes from 8.8.8.8: icmp_seq=2 ttl=117 time=15.218 ms
64 bytes from 8.8.8.8: icmp_seq=3 ttl=117 time=20.398 ms
^C

$ ./client stop 58e1f565-b1d0-436d-8c25-f453408c2514
Done

$ ./client status 58e1f565-b1d0-436d-8c25-f453408c2514
Process status: StatusStopped
Exit status: -1
Signal: 9

$ ./client start ls
Process UUID: f1e30391-9ddb-4578-a48c-b19a6584e79d

$ ./client status f1e30391-9ddb-4578-a48c-b19a6584e79d
Process status: StatusStopped
Exit status: 0

$ ./client stream f1e30391-9ddb-4578-a48c-b19a6584e79d
Makefile
README.md
proto

$ ./client start "ls /bad/name"
Process UUID: 5315aefe-0b81-462f-9fd3-666fc1482caa

$ ./client status 5315aefe-0b81-462f-9fd3-666fc1482caa
Process status: StatusStopped
Exit status: 1

$ ./client stream 5315aefe-0b81-462f-9fd3-666fc1482caa
ls: /bad/name: No such file or directory

```
