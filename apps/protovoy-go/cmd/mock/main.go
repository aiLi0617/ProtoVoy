package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"protovoy.dev/protovoy/internal/buildinfo"
	"protovoy.dev/protovoy/internal/mock"
)

func main() {
	bind := flag.String("bind", "127.0.0.1:39218", "loopback IP address and port")
	rules := flag.String("rules", "", "JSON mock scenario file")
	check := flag.Bool("check", false, "validate the rules file and exit")
	initFile := flag.Bool("init", false, "create an example rules file without overwriting an existing file")
	version := flag.Bool("version", false, "print the application version")
	flag.Parse()
	if *version {
		fmt.Println("ProtoVoy Mock Server " + buildinfo.Version)
		return
	}
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	addr, err := netip.ParseAddrPort(*bind)
	if err != nil || !addr.Addr().IsLoopback() || addr.Addr().Zone() != "" || addr.Port() == 0 {
		log.Fatal("-bind requires a loopback IP and nonzero port")
	}
	explicitRules := *rules != ""
	if *rules == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal("could not locate the configuration directory; set -rules")
		}
		*rules = filepath.Join(dir, "protovoy", "mock-rules.json")
	}
	if *initFile {
		data, err := mock.Encode([]mock.Rule{{
			ID: "hello", Name: "Hello", ProjectKey: "default-project", ServiceKey: "default",
			Method: "GET", Path: "/hello", Status: 200, Body: "Hello",
		}})
		if err != nil {
			log.Fatal(err)
		}
		file, err := os.OpenFile(*rules, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			log.Fatal(err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Example mock rules created")
		return
	}
	if *check || explicitRules {
		if err := mock.ValidateFile(*rules); err != nil {
			log.Fatal(err)
		}
	}
	if *check {
		fmt.Println("Mock rules are valid")
		return
	}
	handler, err := mock.NewHandler(*rules, addr.String(), buildinfo.Version)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: addr.String(), Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	select {
	case err := <-stopped:
		log.Fatal(err)
	case <-ctx.Done():
		deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(deadline); err != nil {
			log.Fatal(err)
		}
		if err := <-stopped; !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}
