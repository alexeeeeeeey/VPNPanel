package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"vpnpanel/frontend"
	"vpnpanel/internal/access"
	"vpnpanel/internal/buildinfo"
	"vpnpanel/internal/cluster"
	"vpnpanel/internal/httpapi"
	"vpnpanel/internal/metrics"
	"vpnpanel/internal/store"
	"vpnpanel/internal/subscription"
	"vpnpanel/internal/update"
	"vpnpanel/internal/validation"
	"vpnpanel/internal/xray"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listen := flag.String("listen", "127.0.0.1:8090", "HTTP listen address (loopback only in this development version)")
	dbPath := flag.String("db", "data/panel.db", "SQLite database")
	web := flag.String("web", "", "external frontend directory (default: embedded frontend)")
	base := flag.String("base-path", "", "panel URL prefix")
	export := flag.String("connection-url", "", "print a connection code for this URL and exit")
	nodeName := flag.String("node-name", "", "node name for the connection code")
	xrayBinary := flag.String("xray", "", "path to Xray executable for automatic configuration deployment")
	xrayAPI := flag.String("xray-api", "", "loopback Xray statistics API address")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if *xrayAPI != "" {
		apiHost, _, err := net.SplitHostPort(*xrayAPI)
		if err != nil || !net.ParseIP(apiHost).IsLoopback() {
			log.Fatal("Xray API must use a loopback IP")
		}
	}
	prefix := strings.TrimRight(*base, "/")
	if prefix != "" && !validation.Path.MatchString(prefix) {
		log.Fatal("invalid base path")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("development version only supports a loopback listen address")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer s.DB.Close()
	identity, err := s.Identity()
	if err != nil {
		log.Fatal(err)
	}
	if *nodeName != "" {
		if err := s.SetNodeName(ctx, *nodeName); err != nil {
			log.Fatal(err)
		}
		identity.Name = *nodeName
	}
	if *export != "" {
		code, err := cluster.ConnectionCode(identity.ID, identity.Name, strings.TrimRight(*export, "/"), prefix)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(code)
		return
	}
	var webFS fs.FS
	if *web != "" {
		webFS = os.DirFS(*web)
	} else {
		webFS, err = frontend.Assets()
		if err != nil {
			log.Fatal(err)
		}
	}
	if _, err := fs.Stat(webFS, "index.html"); err != nil {
		log.Fatalf("frontend index.html: %v", err)
	}
	// Stop background workers before closing SQLite.
	ctx, stop := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { stop(); workers.Wait() }()
	runtime := xray.NewRuntime()
	collector := &xray.Collector{}
	subscriptions := subscription.Start(ctx, s)
	executable, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	updater := update.New(buildinfo.Version, executable)
	sample := metrics.NewSampler(ctx, filepath.Dir(*dbPath), s)
	r := httpapi.New(httpapi.Dependencies{
		Store:         s,
		Subscriptions: subscriptions,
		Sample:        sample,
		Updater:       updater,
		Context:       ctx,
		Runtime:       runtime,
		Collector:     collector,
	})
	httpapi.MountFrontend(r, webFS, prefix)
	handler, err := access.New(r, prefix)
	if err != nil {
		log.Fatal(err)
	}
	workers.Go(func() { runtime.Run(ctx, s, *xrayBinary, filepath.Join(filepath.Dir(*dbPath), "runtime")) })
	workers.Go(func() { collector.Run(ctx, s, *xrayBinary, *xrayAPI) })
	workers.Go(func() { cluster.Run(ctx, s) })
	workers.Go(func() { updater.Run(ctx) })
	srv := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("VPNPanel: http://%s%s/", *listen, prefix)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- srv.ListenAndServe() }()
	restart := false
	select {
	case <-ctx.Done():
	case <-updater.Restart():
		restart = true
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server: %v", err)
		}
	}
	shutdown, finish := context.WithTimeout(context.Background(), 15*time.Second)
	defer finish()
	if err := srv.Shutdown(shutdown); err != nil {
		_ = srv.Close()
	}
	stop()
	workers.Wait()
	updater.Wait()
	subscriptions.Close()
	_ = s.DB.Close()
	if restart {
		if err := update.RestartProcess(executable); err != nil {
			log.Fatal(err)
		}
	}
}
