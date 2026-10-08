//go:build windows

package browser

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// setProcGroup is a no-op on Windows (process groups work differently).
func setProcGroup(cmd *exec.Cmd) {
	// Windows does not use Unix process groups.
	// Browser child processes will be terminated via Process.Kill().
}

// killProcessTree terminates a launched browser on Windows.
func killProcessTree(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// tieToEngine makes a browser that has just been started die when this process
// does, however this process ends.
//
// Close is the only thing that stops a launched browser, and a process that is
// killed never reaches it. The browser stayed behind, still listening on the
// debugging port, and the next launch on that port failed with "CDP not
// reachable" against a browser nobody was driving.
//
// A job object marked kill-on-close does what no handler in this process can:
// Windows closes the job's last handle when the process goes, for any reason,
// and takes the job's members with it. Failure here is not reported — the
// browser runs either way, and an engine that exits normally still closes it.
func tieToEngine(cmd *exec.Cmd) {
	job := engineJob()
	if job == 0 {
		return
	}
	const processSetQuota, processTerminate = 0x0100, 0x0001
	proc, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(proc)
	_, _, _ = procAssignProcessToJobObject.Call(uintptr(job), uintptr(proc))
}

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")

	jobOnce   sync.Once
	jobHandle syscall.Handle
)

// engineJob returns the one job every launched browser joins, creating it on
// first use. Its handle is never closed: closing it is the signal.
func engineJob() syscall.Handle {
	jobOnce.Do(func() {
		h, _, _ := procCreateJobObjectW.Call(0, 0)
		if h == 0 {
			return
		}
		const (
			jobObjectExtendedLimitInformationClass = 9
			jobObjectLimitKillOnJobClose           = 0x2000
		)
		info := jobObjectExtendedLimitInformation{}
		info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
		ok, _, _ := procSetInformationJobObject.Call(h, jobObjectExtendedLimitInformationClass,
			uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		if ok == 0 {
			_ = syscall.CloseHandle(syscall.Handle(h))
			return
		}
		jobHandle = syscall.Handle(h)
	})
	return jobHandle
}

// JOBOBJECT_EXTENDED_LIMIT_INFORMATION and the structures inside it, laid out
// as winnt.h has them.
type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}
