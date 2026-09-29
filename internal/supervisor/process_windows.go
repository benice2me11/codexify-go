//go:build windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func attachProcess(cmd *exec.Cmd) (func(), error) {
	if cmd.Process == nil {
		return nil, errors.New("process has not started")
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create process job: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	ret, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil || ret == 0 {
		windows.CloseHandle(job)
		if err != nil {
			return nil, fmt.Errorf("configure process job: %w", err)
		}
		return nil, errors.New("configure process job: SetInformationJobObject failed")
	}

	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("open process for job assignment: %w", err)
	}
	defer windows.CloseHandle(process)

	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("assign process to job: %w", err)
	}

	return func() {
		_ = windows.CloseHandle(job)
	}, nil
}

func stopProcessTree(ctx context.Context, pid int) error {
	cmd := exec.CommandContext(ctx, "taskkill.exe", "/T", "/PID", strconv.Itoa(pid))
	configureProcess(cmd)
	return cmd.Run()
}

func forceKillProcessTree(pid int) error {
	cmd := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid))
	configureProcess(cmd)
	return cmd.Run()
}

func waitProcessTreeExit(context.Context, int) error { return nil }
