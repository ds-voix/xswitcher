// Package ipc — see protocol.go for overview.
// client.go provides a dial-and-request helper for the GUI side.
package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
)

// Client connects to the daemon over a Unix domain socket.
type Client struct {
	path string
}

// NewClient creates an IPC client targeting the given socket path.
func NewClient(path string) *Client {
	return &Client{path: path}
}

// Send sends a single request and returns the daemon's response.
func (c *Client) Send(req Request) (Response, error) {
	conn, err := net.Dial("unix", c.path)
	if err != nil {
		return Response{}, fmt.Errorf("ipc: dial %s: %w", c.path, err)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("ipc: marshal request: %w", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return Response{}, fmt.Errorf("ipc: write: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return Response{}, fmt.Errorf("ipc: read: %w", err)
		}
		return Response{}, fmt.Errorf("ipc: no response from daemon")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return Response{}, fmt.Errorf("ipc: unmarshal response: %w", err)
	}
	return resp, nil
}

// Ping checks whether the daemon is reachable.
func (c *Client) Ping() error {
	resp, err := c.Send(Request{Cmd: "ping"})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("daemon returned error: %s", resp.Error)
	}
	return nil
}

// Status retrieves current daemon status.
func (c *Client) Status() (Response, error) {
	return c.Send(Request{Cmd: "status"})
}

// Reload asks the daemon to reload its configuration via respawn.
func (c *Client) Reload() (Response, error) {
	return c.Send(Request{Cmd: "reload"})
}

// GetConfig retrieves the daemon's current config file as a nested map.
func (c *Client) GetConfig() (map[string]any, error) {
	resp, err := c.Send(Request{Cmd: "get_config"})
	if err != nil {
		return nil, err
	}
	if !resp.Ok {
		return nil, fmt.Errorf("ipc: get_config: %s", resp.Error)
	}
	return resp.Data, nil
}

// SetConfig sends dotted-key modifications (e.g. "Keyboard.Delay") to the
// daemon, which validates the resulting config before writing it.
func (c *Client) SetConfig(mods map[string]any) error {
	resp, err := c.Send(Request{Cmd: "set_config", Data: mods})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("ipc: set_config: %s", resp.Error)
	}
	return nil
}

// Validate asks the daemon to re-parse its config file without applying it.
func (c *Client) Validate() error {
	resp, err := c.Send(Request{Cmd: "validate"})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("ipc: validate: %s", resp.Error)
	}
	return nil
}

// GetDiary retrieves the diary content tail from the daemon. The daemon
// owns the diary file (it may live in a root-only location), so the GUI
// reads it exclusively through IPC.
func (c *Client) GetDiary() (string, error) {
	resp, err := c.Send(Request{Cmd: "get_diary"})
	if err != nil {
		return "", err
	}
	if !resp.Ok {
		return "", fmt.Errorf("ipc: get_diary: %s", resp.Error)
	}
	return resp.Diary, nil
}

// Subscribe opens a dedicated connection, registers for daemon-pushed
// events and forwards them to onEvent on a background goroutine. The
// returned channel is closed when the connection dies (daemon respawn,
// crash), so the caller can re-subscribe in a loop. The caller keeps
// normal request/response traffic on Send.
func (c *Client) Subscribe(onEvent func(Event)) (<-chan struct{}, error) {
	done := make(chan struct{})
	conn, err := net.Dial("unix", c.path)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial %s: %w", c.path, err)
	}
	data, err := json.Marshal(Request{Cmd: "subscribe"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ipc: marshal subscribe: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ipc: subscribe write: %w", err)
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		conn.Close()
		return nil, fmt.Errorf("ipc: subscribe: no acknowledgement")
	}
	var ack Response
	if err := json.Unmarshal(scanner.Bytes(), &ack); err != nil || !ack.Ok {
		conn.Close()
		return nil, fmt.Errorf("ipc: subscribe rejected")
	}
	go func() {
		defer close(done)
		defer conn.Close()
		for scanner.Scan() {
			var ev Event
			if json.Unmarshal(scanner.Bytes(), &ev) == nil && ev.Event != "" {
				onEvent(ev)
			}
		}
	}()
	return done, nil
}
