package update

import (
	"os"
	"syscall"
)

func RestartProcess(executable string) error {
	return syscall.Exec(executable, os.Args, os.Environ())
}
