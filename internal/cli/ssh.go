package cli

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func Execute(host string, port int, prefix, authFile string, args []string) error {
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n@/") || port < 1 || port > 65535 {
		return errors.New("specify a valid SSH alias and API port\n" + Usage)
	}
	if !basePath.MatchString(prefix) {
		return errors.New("invalid base path")
	}
	if !authPath.MatchString(authFile) {
		return errors.New("specify an absolute remote auth-file path without spaces or shell characters")
	}
	method, path, payload, err := request(args)
	if err != nil {
		return err
	}
	// Only fixed route strings, a numeric port and validated UUIDs enter the remote shell command.
	remote := "curl --silent --show-error --fail-with-body --max-time 20 --noproxy '*' -X " + method
	remote += " --header @" + authFile
	if payload != nil {
		remote += " -H 'Content-Type: application/json' -H 'X-Panel-Request: 1' --data-binary @-"
	}
	remote += " http://127.0.0.1:" + strconv.Itoa(port) + prefix + "/api/" + path
	cmd := exec.Command("ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2", "--", host, remote)
	cmd.Stdin = strings.NewReader(string(payload))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
