// Package ipc — see protocol.go for overview.
// server.go implements the Unix domain socket listener for the daemon side.
package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

const (
	// DefaultSocketPath is where the daemon listens for GUI connections.
	DefaultSocketPath = "/run/xswitcher/xswitcher.sock"
	socketPerm        = 0660
)

// StatusFunc returns current daemon status for the "status" command.
type StatusFunc func() Response

// ReloadFunc triggers a config reload (respawn) for the "reload" command.
// It returns a Response indicating success or failure.
type ReloadFunc func() Response

// Server listens on a Unix domain socket and dispatches incoming commands.
type Server struct {
	path     string
	statusFn StatusFunc
	reloadFn ReloadFunc
	listener net.Listener
	mu       sync.Mutex
}

// NewServer creates an IPC server bound to the given socket path.
func NewServer(path string, statusFn StatusFunc, reloadFn ReloadFunc) *Server {
	return &Server{
		path:     path,
		statusFn: statusFn,
		reloadFn: reloadFn,
	}
}

// Start begins listening in the background. It removes any stale socket file
// first. The caller should call Stop() when shutting down.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ipc: remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("ipc: listen: %w", err)
	}

	if err := os.Chmod(s.path, socketPerm); err != nil {
		ln.Close()
		return fmt.Errorf("ipc: chmod socket: %w", err)
	}

	s.listener = ln
	go s.acceptLoop(ln)
	return nil
}

// Stop closes the listener and removes the socket file.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	os.Remove(s.path)
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed during shutdown.
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			resp := Response{Ok: false, Error: fmt.Sprintf("invalid request: %v", err)}
			writeResponse(conn, resp)
			continue
		}
		resp := s.dispatch(req)
		writeResponse(conn, resp)
	}
}

func (s *Server) dispatch(req Request) Response {
	switch req.Cmd {
	case "ping":
		return Response{Ok: true}
	case "status":
		if s.statusFn != nil {
			return s.statusFn()
		}
		return Response{Ok: false, Error: "status not available"}
	case "reload":
		if s.reloadFn != nil {
			return s.reloadFn()
		}
		return Response{Ok: false, Error: "reload not available"}
	default:
		return Response{Ok: false, Error: fmt.Sprintf("unknown command: %q", req.Cmd)}
	}
}

func writeResponse(w net.Conn, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	data = append(data, '\n')
	w.Write(data)
}