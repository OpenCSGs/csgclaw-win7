//go:build windows

package processgroup

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// Windows 7 cannot nest our command job inside an inherited launcher or PCA job.
// Leave that job only when its policy explicitly permits it, then assign the
// suspended child to our normal owned job before any command can execute.
func prepareInheritedJob(cmd *exec.Cmd) error {
	version := windows.RtlGetVersion()
	if version.MajorVersion > 6 || version.MajorVersion == 6 && version.MinorVersion >= 2 {
		return nil
	}
	var inJob uint32
	ok, _, err := isProcessInJob.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return fmt.Errorf("inspect inherited Windows job: %w", err)
	}
	if inJob == 0 {
		return nil
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		return fmt.Errorf("inspect inherited Windows job limits: %w", err)
	}
	flags := limits.BasicLimitInformation.LimitFlags
	if flags&windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK != 0 {
		return nil
	}
	if flags&windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK == 0 {
		return fmt.Errorf("Windows 7 inherited job does not allow child breakaway; start CSGClaw outside the restricting launcher: %w", windows.ERROR_ACCESS_DENIED)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_BREAKAWAY_FROM_JOB
	return nil
}
