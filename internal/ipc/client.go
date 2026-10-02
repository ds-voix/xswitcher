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