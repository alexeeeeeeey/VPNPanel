//go:build !linux

package update

import "errors"

func RestartProcess(string) error {
	return errors.New("автоматическая замена бинарника поддерживается только на Linux")
}
