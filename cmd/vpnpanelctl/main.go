package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"vpnpanel/internal/buildinfo"
	"vpnpanel/internal/cli"
)

func main() {
	host := flag.String("host", "", "host alias from OpenSSH config")
	port := flag.Int("port", 8090, "loopback API port on the server")
	prefix := flag.String("base-path", "", "panel URL prefix")
	authFile := flag.String("auth-file", "/etc/vpnpanel/cli.headers", "node-local headers file read by curl on the SSH server")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), cli.Usage) }

	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if err := cli.Execute(*host, *port, strings.TrimRight(*prefix, "/"), *authFile, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
