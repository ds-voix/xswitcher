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

// ConfigFunc reads the current config file content for "get_config".
type ConfigFunc func() (map[string]any, error)

// SetConfigFunc applies modifications and validates for "set_config".
type SetConfigFunc func(mods map[string]any) error

// ValidateFunc performs a dry-run parse of the current config for "validate".
type ValidateFunc func() error

// DiaryFunc returns the current diary content for "get_diary".
type DiaryFunc func() (string, error)

// connWriter serialises every write to one connection: dispatch responses
// and Broadcast pushes run on different goroutines and their lines must not
// interleave.
type connWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

func (w *connWriter) writeLine(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.conn.Write(data)
	return err
}

func (w *connWriter) write(resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = w.writeLine(append(data, '\n'))
}

// Server listens on a Unix domain socket and dispatches incoming commands.
type Server struct {
	path        string
	statusFn    StatusFunc
	reloadFn    ReloadFunc
	configFn    ConfigFunc
	setConfigFn SetConfigFunc
	validateFn  ValidateFunc
	diaryFn     DiaryFunc
	listener    net.Listener
	mu          sync.Mutex
	conns       map[net.Conn]*connWriter // every live connection's shared writer
	subs        map[*connWriter]struct{} // connections that sent "subscribe"
}

// NewServer creates an IPC server bound to the given socket path.
func NewServer(path string, statusFn StatusFunc, reloadFn ReloadFunc) *Server {
	return &Server{
		path:     path,
		statusFn: statusFn,
		reloadFn: reloadFn,
		conns:    make(map[net.Conn]*connWriter),
		subs:     make(map[*connWriter]struct{}),
	}
}

// SetConfigHandler registers the callback for get_config/set_config/validate commands.
func (s *Server) SetConfigHandler(cfgFn ConfigFunc, setFn SetConfigFunc, valFn ValidateFunc) {
	s.configFn = cfgFn
	s.setConfigFn = setFn
	s.validateFn = valFn
}

// SetDiaryHandler registers the callback for the "get_diary" command.
func (s *Server) SetDiaryHandler(fn DiaryFunc) {
	s.diaryFn = fn
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
	w := &connWriter{conn: conn}
	s.mu.Lock()
	s.conns[conn] = w
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		delete(s.subs, w)
		s.mu.Unlock()
		conn.Close()
	}()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			w.write(Response{Ok: false, Error: fmt.Sprintf("invalid request: %v", err)})
			continue
		}
		if req.Cmd == "subscribe" {
			s.mu.Lock()
			s.subs[w] = struct{}{}
			s.mu.Unlock()
			w.write(Response{Ok: true})
			continue
		}
		w.write(s.dispatch(req))
	}
}

// Broadcast pushes an event line to every subscribed connection. A dead
// subscription is dropped on the spot; handleConn cleans the connection up
// when its read loop notices the same error. The daemon's hot path only
// pays a map snapshot here — the conn write locks are taken outside s.mu.
func (s *Server) Broadcast(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.mu.Lock()
	writers := make([]*connWriter, 0, len(s.subs))
	for w := range s.subs {
		writers = append(writers, w)
	}
	s.mu.Unlock()
	for _, w := range writers {
		if err := w.writeLine(data); err != nil {
			s.mu.Lock()
			delete(s.subs, w)
			s.mu.Unlock()
		}
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
	case "get_config":
		if s.configFn == nil {
			return Response{Ok: false, Error: "get_config not available"}
		}
		data, err := s.configFn()
		if err != nil {
			return Response{Ok: false, Error: err.Error()}
		}
		return Response{Ok: true, Data: data}
	case "set_config":
		if s.setConfigFn == nil {
			return Response{Ok: false, Error: "set_config not available"}
		}
		if err := s.setConfigFn(req.Data); err != nil {
			return Response{Ok: false, Error: err.Error()}
		}
		return Response{Ok: true}
	case "validate":
		if s.validateFn == nil {
			return Response{Ok: false, Error: "validate not available"}
		}
		if err := s.validateFn(); err != nil {
			return Response{Ok: false, Error: err.Error()}
		}
		return Response{Ok: true}
	case "get_diary":
		if s.diaryFn == nil {
			return Response{Ok: false, Error: "diary is not enabled"}
		}
		text, err := s.diaryFn()
		if err != nil {
			return Response{Ok: false, Error: err.Error()}
		}
		return Response{Ok: true, Diary: text}
	default:
		return Response{Ok: false, Error: fmt.Sprintf("unknown command: %q", req.Cmd)}
	}
}
