package xray

import (
	"os/exec"
	"runtime"
	"syscall"
)

func bindXraySupervisor() func() {
	// Pdeathsig follows the creating OS thread, not just the parent PID.
	// Keep that thread alive until the supervisor has stopped its children.
	runtime.LockOSThread()
	return runtime.UnlockOSThread
}

func configureManagedXray(command *exec.Cmd) {
	// A panel killed without running its cleanup must not leave a core holding
	// the listener ports and serving an obsolete configuration after recovery.
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
