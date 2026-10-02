// Package ipc implements the line-delimited JSON protocol used by the xswitcher
// daemon and GUI to communicate over a Unix domain socket.
package ipc

// Request is sent from the GUI to the daemon. Each request is a single JSON
// object on one line, terminated by '\n'.
type Request struct {
	Cmd  string         `json:"cmd"`            // "ping", "status", "reload", "get_config", "set_config", "validate"
	Data map[string]any `json:"data,omitempty"` // key-path modifications for set_config
}

// Response is sent from the daemon to the GUI. Always includes Ok; on failure
// Error carries a human-readable message.
type Response struct {
	Ok      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	PID     int            `json:"pid,omitempty"`
	Uptime  string         `json:"uptime,omitempty"`
	Config  string         `json:"config,omitempty"`
	Devices []string       `json:"devices,omitempty"`
	Data    map[string]any `json:"data,omitempty"` // raw TOML content or parsed config for get_config
}
