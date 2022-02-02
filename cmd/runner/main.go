package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// TODO: use service config for
// (1) setting CPU and memory resource limits.
// (2) including block device numbers (currently omitted) and setting associated bandwidth limits
const (
	cpuShares = 512 // CPU shares
	rssLimit  = 10  // memory limit with MB
)

func main() {
	var err error

	switch os.Args[1] {
	case "start":
		err = start()
	case "cgr":
		err = cgr()
	default:
		err = fmt.Errorf("invalid command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

func start() error {
	cmd := exec.Command("/proc/self/exe", append([]string{"cgr"}, os.Args[2:]...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	check(cmd.Run())

	return nil
}

func cgr() error {
	cgroupName := os.Args[2]
	cgroupMemDir := filepath.Join("/sys/fs/cgroup/memory", cgroupName)
	cgroupMemLimitFile := filepath.Join(cgroupMemDir, "memory.limit_in_bytes")
	cgroupMemProcsFile := filepath.Join(cgroupMemDir, "cgroup.procs")

	cgroupCpuDir := filepath.Join("/sys/fs/cgroup/cpu", cgroupName)
	cgroupCpuSharesFile := filepath.Join(cgroupCpuDir, "cpu.shares")
	cgroupCpuProcsFile := filepath.Join(cgroupCpuDir, "cgroup.procs")

	// create cgroup directories
	if err := os.MkdirAll(cgroupMemDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(cgroupCpuDir, 0755); err != nil {
		return err
	}
	// set memory limit
	if err := writeInt(cgroupMemLimitFile, rssLimit*1024*1024); err != nil {
		return err
	}
	// set CPU shares
	if err := writeInt(cgroupCpuSharesFile, cpuShares); err != nil {
		return err
	}
	// set cgroup procs id
	if err := writeInt(cgroupMemProcsFile, os.Getpid()); err != nil {
		return err
	}
	if err := writeInt(cgroupCpuProcsFile, os.Getpid()); err != nil {
		return err
	}

	cmd := exec.Command(os.Args[3], os.Args[4:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	check(cmd.Run())

	return nil
}

func writeInt(path string, value int) error {
	return ioutil.WriteFile(path, []byte(fmt.Sprintf("%d", value)), 0755)
}

func check(err error) {
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ProcessState.ExitCode())
		} else {
			os.Exit(1)
		}
	}
}
