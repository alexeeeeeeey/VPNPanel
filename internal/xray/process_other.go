//go:build !linux

package xray

import "os/exec"

func bindXraySupervisor() func() { return func() {} }

func configureManagedXray(command *exec.Cmd) {}
