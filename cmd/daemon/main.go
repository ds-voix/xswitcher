// Package main is a thin wrapper around the xswitcher daemon core (src/) that
// starts the IPC server after config parsing so the GUI can connect.
//
// Build:
//
//	go build -o bin/xswitcher ./cmd/daemon/
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"xswitcher/internal/ipc"
)

func main() {
	// The real daemon logic lives in src/xswitcher.go and is linked via the
	// import side-effect of the src package. This entry point only adds the
	// IPC listener on top of the existing main() flow.
	//
	// NOTE: In the current architecture, src/xswitcher.go has its own main()
	// which calls config(), sequences(), keys(), connectEvents(), etc.
	// This cmd/daemon/main.go is intended to eventually replace that main()
	// by importing src as a library. For Phase 1 MVP, the IPC server goroutine
	// is started directly from src/xswitcher.go after line 2101 instead.
	//
	// This file exists as a placeholder for the future thin-wrapper architecture.
	// The actual IPC integration happens in src/xswitcher.go for now.

	sockPath := ipc.DefaultSocketPath
	if envSock, ok := os.LookupEnv("XSWITCHER_SOCK"); ok {
		sockPath = envSock
	}

	srv := ipc.NewServer(sockPath, nil, nil)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "IPC server failed to start: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop()

	fmt.Printf("IPC server listening on %s\n", sockPath)

	// Wait for termination signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Shutting down IPC server.")
}
