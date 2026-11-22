package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestartProcess(t *testing.T) {
	if target := os.Getenv("VPNPANEL_TEST_EXEC_TARGET"); target != "" {
		if err := RestartProcess(target); err != nil {
			t.Fatal(err)
		}
		return
	}
	target := filepath.Join(t.TempDir(), "replacement")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s|%s|%s' \"$VPNPANEL_TEST_VALUE\" \"$1\" \"$$\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestRestartProcess$")
	command.Env = append(os.Environ(), "VPNPANEL_TEST_EXEC_TARGET="+target, "VPNPANEL_TEST_VALUE=preserved")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("preserved|-test.run=^TestRestartProcess$|%d", command.Process.Pid)
	if strings.TrimSpace(string(output)) != want {
		t.Fatalf("restart did not preserve environment, arguments or PID: %q, want %q", output, want)
	}
}
