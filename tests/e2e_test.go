package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var workDir string

func init() {
	workDir, _ = filepath.Abs("..")
}
func TestMain(m *testing.M) {
	srv := exec.Cmd{
		Dir:  workDir,
		Path: "./server",
		Env:  os.Environ(),
	}

	fmt.Println("Starting server")
	err := srv.Start()
	if err != nil {
		fmt.Printf("failed to start server: %v\n", err)
		os.Exit(1)
	}

	// allow server to start
	time.Sleep(time.Second)

	statusChan := make(chan interface{}, 1)

	go func() {
		err := srv.Wait()
		if err != nil {
			statusChan <- err
		}
	}()

	go func() {
		statusChan <- m.Run()
	}()

	select {
	case x := <-statusChan:
		switch v := x.(type) {
		case int:
			fmt.Printf("completed tests\n")
			srv.Process.Kill()
			os.Exit(v)
		case error:
			fmt.Printf("server error: %v\n", v)
			os.Exit(1)
		}
	}
}

func TestSyncApp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// start process
	err := getClnCmd([]string{"start", "echo", "HelloWorld"}, &stdout, &stderr, 1).Run()

	txt := string(stdout.Bytes())
	require.NoError(t, err, "start error[%v] stdout[%s] stderr[%s]", err, txt, string(stderr.Bytes()))

	var uid string
	if indx := strings.Index(txt, "Process UID:"); indx != -1 {
		uid = strings.TrimSpace(txt[(indx + 12):])
	}
	require.NotEmpty(t, uid, "no uid in stdout[%s]", txt)

	// get process status
	stdout.Reset()
	stderr.Reset()

	err = getClnCmd([]string{"status", uid}, &stdout, &stderr, 1).Run()
	require.NoError(t, err, "status error[%v] stdout[%s] stderr[%s]", err, string(stdout.Bytes()), string(stderr.Bytes()))

	txt = strings.TrimSpace(string(stdout.Bytes()))
	require.Equal(t, txt, "Process status: StatusStopped\nExit status: 0", "unexpected output [%s]", txt)

	// get process output
	stdout.Reset()
	stderr.Reset()

	err = getClnCmd([]string{"stream", uid}, &stdout, &stderr, 1).Run()
	require.NoError(t, err, "stream error[%v] stdout[%s] stderr[%s]", err, string(stdout.Bytes()), string(stderr.Bytes()))

	txt = strings.TrimSpace(string(stdout.Bytes()))
	require.Equal(t, txt, "HelloWorld")
}

func TestAsyncApp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// start process
	err := getClnCmd([]string{"start", "scripts/loop.sh"}, &stdout, &stderr, 1).Run()

	txt := string(stdout.Bytes())
	require.NoError(t, err, "start error[%v] stdout[%s] stderr[%s]", err, txt, string(stderr.Bytes()))

	var uid string
	if indx := strings.Index(txt, "Process UID:"); indx != -1 {
		uid = strings.TrimSpace(txt[(indx + 12):])
	}
	require.NotEmpty(t, uid, "no uid in stdout[%s]", txt)

	// get process status
	stdout.Reset()
	stderr.Reset()

	err = getClnCmd([]string{"status", uid}, &stdout, &stderr, 1).Run()
	require.NoError(t, err, "status error[%v] stdout[%s] stderr[%s]", err, string(stdout.Bytes()), string(stderr.Bytes()))

	txt = strings.TrimSpace(string(stdout.Bytes()))
	require.Equal(t, txt, "Process status: StatusRunning", "unexpected output [%s]", txt)

	// get process output
	var stdout1, stderr1 bytes.Buffer
	streamClient := getClnCmd([]string{"stream", uid}, &stdout1, &stderr1, 1)
	err = streamClient.Start()
	require.NoError(t, err, "stream error[%v] stdout[%s] stderr[%s]", err, string(stdout1.Bytes()), string(stderr1.Bytes()))

	stop := make(chan interface{}, 1)
	go func() {
		stop <- streamClient.Wait()
	}()

	// stop process
	stdout.Reset()
	stderr.Reset()

	err = getClnCmd([]string{"stop", uid}, &stdout, &stderr, 1).Run()
	require.NoError(t, err, "stop error[%v] stdout[%s] stderr[%s]", err, string(stdout.Bytes()), string(stderr.Bytes()))

	// validate streaming has stopped
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-stop:
		timer.Stop()
	case <-timer.C:
		// streaming didn't stop: kill the process
		streamClient.Process.Kill()
		t.Fatalf("streaming did not stop")
	}

	// get process status
	stdout.Reset()
	stderr.Reset()

	err = getClnCmd([]string{"status", uid}, &stdout, &stderr, 1).Run()
	require.NoError(t, err, "status error[%v] stdout[%s] stderr[%s]", err, string(stdout.Bytes()), string(stderr.Bytes()))

	txt = strings.TrimSpace(string(stdout.Bytes()))
	require.Equal(t, txt, "Process status: StatusStopped\nExit status: -1\nSignal: 9", "unexpected output [%s]", txt)
}

func getClnCmd(args []string, stdout, stderr *bytes.Buffer, clientN int) *exec.Cmd {
	env := append(os.Environ(), []string{"CA_CERT=./certs/ca.crt", fmt.Sprintf("CLIENT_CERT=./certs/client%d.crt", clientN), fmt.Sprintf("CLIENT_KEY=./certs/client%d.key", clientN)}...)
	return &exec.Cmd{
		Dir:    workDir,
		Path:   "./client",
		Args:   append([]string{"./client"}, args...),
		Env:    env,
		Stdout: stdout,
		Stderr: stderr,
	}
}
