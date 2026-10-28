//go:build windows

package ocr

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const platformSupported = true

// jobObject holds the worker processes of one pool. Limits:
// JOB_OBJECT_LIMIT_PROCESS_MEMORY (per process, Config.MaxMem),
// KILL_ON_JOB_CLOSE (the workers die with the pool or with the server) and
// DIE_ON_UNHANDLED_EXCEPTION (a crash in native code ends the process at
// once instead of waiting on the Windows error reporting).
type jobObject struct {
	mu sync.Mutex
	h  windows.Handle
}

func newJob(maxMem int64) (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("ocr: CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
		windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	info.ProcessMemoryLimit = uintptr(maxMem)
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("ocr: SetInformationJobObject: %w", err)
	}
	return &jobObject{h: h}, nil
}

// prepare makes the child start suspended (so that it runs no instruction
// before it is inside the job) and without a console window.
func (j *jobObject) prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW,
	}
}

// adopt assigns the suspended process pid to the job and only then resumes
// its threads. On error the caller kills the process.
func (j *jobObject) adopt(pid int) error {
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("ocr: OpenProcess: %w", err)
	}
	j.mu.Lock() // held so that close cannot release the handle under us
	if j.h == 0 {
		err = ErrClosed
	} else {
		err = windows.AssignProcessToJobObject(j.h, ph)
	}
	j.mu.Unlock()
	_ = windows.CloseHandle(ph)
	if err == ErrClosed {
		return err
	}
	if err != nil {
		return fmt.Errorf("ocr: AssignProcessToJobObject: %w", err)
	}
	return resumeProcess(uint32(pid))
}

// resumeProcess resumes every thread of pid (a new suspended process has
// exactly one). os/exec closes the primary thread handle, so the threads are
// found through a Toolhelp snapshot.
func resumeProcess(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("ocr: CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	resumed := 0
	for err = windows.Thread32First(snap, &te); err == nil; err = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != pid {
			continue
		}
		th, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
		if err != nil {
			return fmt.Errorf("ocr: OpenThread: %w", err)
		}
		_, err = windows.ResumeThread(th)
		_ = windows.CloseHandle(th)
		if err != nil {
			return fmt.Errorf("ocr: ResumeThread: %w", err)
		}
		resumed++
	}
	if resumed == 0 {
		return fmt.Errorf("ocr: nenhuma thread do worker para retomar")
	}
	return nil
}

// close closes the job handle; KILL_ON_JOB_CLOSE ends every process in it.
func (j *jobObject) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h != 0 {
		_ = windows.CloseHandle(j.h)
		j.h = 0
	}
}
