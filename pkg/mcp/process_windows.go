//go:build windows

package mcp

import (
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	jobMu    sync.Mutex
	procJobs = map[int]windows.Handle{}
)

// ConfigureProcess is a no-op on Windows; the process tree is contained by a
// Job Object assigned right after the process starts.
func ConfigureProcess(cmd *exec.Cmd) {}

// AfterProcessStart assigns the running process to a Job Object configured to
// kill the whole tree when it closes. If that fails, terminateProcessTree
// falls back to taskkill /T.
func AfterProcessStart(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	jobMu.Lock()
	procJobs[cmd.Process.Pid] = job
	jobMu.Unlock()
	return nil
}

// TerminateProcessTree kills the Job Object holding the process tree, or falls
// back to taskkill /T when no job was assigned.
func TerminateProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	jobMu.Lock()
	job := procJobs[pid]
	delete(procJobs, pid)
	jobMu.Unlock()
	if job != 0 {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		return nil
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	_ = k.Run()
	return cmd.Process.Kill()
}
