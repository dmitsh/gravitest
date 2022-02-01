package engine

import (
	"bytes"
	"errors"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/google/uuid"

	"github.com/dmitsh/gravitest/proto"
)

const (
	PermStart  = 0x01
	PermStop   = 0x02
	PermStatus = 0x04
	PermStream = 0x08
)

var (
	ErrProcNotFound = errors.New("process not found")
	ErrPermDenied   = errors.New("permission denied")
)

type Process struct {
	clientID string
	cmd      *exec.Cmd
	output   bytes.Buffer
	status   *proto.Status
}

// TRADE OFF
// Due to time constrains I'm not implementing resilience in case the server crashes or restarts.
// Ideally I should have process table and last generated UID persisted.
type ProcManager struct {
	// process table [process UID : Process]
	procs     map[string]*Process
	procMutex sync.Mutex

	// permission table [client ID : permission bitmap]
	perm map[string]int
}

func NewProcManager() *ProcManager {
	return &ProcManager{
		procs: make(map[string]*Process),
		perm: map[string]int{
			"client1": PermStart | PermStop | PermStatus | PermStream,
			"client2": PermStart | PermStop | PermStream,
		},
	}
}

func (m *ProcManager) generateUID() string {
	return uuid.New().String()
}

func (m *ProcManager) addProcess(uid string, proc *Process) {
	m.procMutex.Lock()
	defer m.procMutex.Unlock()
	m.procs[uid] = proc
}

func (m *ProcManager) GetProcessStatus(uid string) (*proto.Status, error) {
	m.procMutex.Lock()
	defer m.procMutex.Unlock()
	if proc, ok := m.procs[uid]; ok {
		return proc.status, nil
	}
	return nil, ErrProcNotFound
}

func (m *ProcManager) checkPermission(clientID string, ask int) error {
	if perm, ok := m.perm[clientID]; !ok || (perm&ask == 0) {
		return ErrPermDenied
	}
	return nil
}

func (m *ProcManager) StartProcess(clientID, exe string, args ...string) (string, error) {
	if err := m.checkPermission(clientID, PermStart); err != nil {
		return "", err
	}

	uid := m.generateUID()
	proc := &Process{
		clientID: clientID,
		cmd:      exec.Command("./runner", append([]string{"start", "worker-" + uid, exe}, args...)...),
		status: &proto.Status{
			ProcStatus: proto.Status_StatusNotStarted,
		},
	}

	proc.cmd.Stdout = &proc.output
	proc.cmd.Stderr = &proc.output
	proc.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	m.addProcess(uid, proc)

	go func() {
		err := proc.cmd.Start()
		if err != nil {
			log.Printf("failed to start %q : %v", strings.Join(append([]string{exe}, args...), " "), err)
			return
		}
		m.procMutex.Lock()
		proc.status.ProcStatus = proto.Status_StatusRunning
		m.procMutex.Unlock()

		err = proc.cmd.Wait()
		m.procMutex.Lock()
		proc.status.ProcStatus = proto.Status_StatusStopped

		if exitErr, ok := err.(*exec.ExitError); ok {
			proc.status.ExitStatus = int32(exitErr.ProcessState.ExitCode())
			if osStatus, ok := proc.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && osStatus.Signaled() {
				proc.status.Signal = int32(osStatus.Signal())
			}
		} else {
			// TRADE OFF
			// not an exit error: set the exit code to 1 and log the error
			proc.status.ExitStatus = 1
			log.Printf("failed to run %q : %v", strings.Join(append([]string{exe}, args...), " "), err)
		}
		m.procMutex.Unlock()
	}()

	return uid, nil
}

func (m *ProcManager) StopProcess(clientID, uid string) error {
	if err := m.checkPermission(clientID, PermStop); err != nil {
		return err
	}
	m.procMutex.Lock()
	defer m.procMutex.Unlock()
	proc, ok := m.procs[uid]
	if !ok || proc.clientID != clientID {
		return ErrProcNotFound
	}
	if proc.status.ProcStatus == proto.Status_StatusRunning {
		return syscall.Kill(-proc.cmd.Process.Pid, syscall.SIGKILL)
	}
	return nil
}

func (m *ProcManager) StatusProcess(clientID, uid string) (*proto.Status, error) {
	if err := m.checkPermission(clientID, PermStatus); err != nil {
		return nil, err
	}
	m.procMutex.Lock()
	defer m.procMutex.Unlock()
	proc, ok := m.procs[uid]
	if !ok || proc.clientID != clientID {
		return nil, ErrProcNotFound
	}
	return proc.status, nil
}

func (m *ProcManager) StreamOutput(clientID, uid string) (*bytes.Buffer, error) {
	if err := m.checkPermission(clientID, PermStream); err != nil {
		return nil, err
	}
	m.procMutex.Lock()
	defer m.procMutex.Unlock()
	proc, ok := m.procs[uid]
	if !ok || proc.clientID != clientID {
		return nil, ErrProcNotFound
	}
	return &proc.output, nil
}
