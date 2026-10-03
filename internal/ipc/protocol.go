// Package ipc implements the line-delimited JSON protocol used by the xswitcher
// daemon and GUI to communicate over a Unix domain socket.
package ipc

// Request is sent from the GUI to the daemon. Each request is a single JSON
// object on one line, terminated by '\n'.
type Request struct {
	Cmd  string         `json:"cmd"`            // "ping", "status", "reload", "get_config", "set_config", "validate", "subscribe", "get_diary"
	Data map[string]any `json:"data,omitempty"` // key-path modifications for set_config
}

// Event is a daemon-initiated line pushed to subscribed connections
// (after "subscribe" is acknowledged). It never carries user text: Name
// is only an event kind.
type Event struct {
	Event string `json:"event"`          // "sound"
	Name  string `json:"name,omitempty"` // "layout_switch", "retype", "auto_replace", "error"
}

// Response is sent from the daemon to the GUI. Always includes Ok; on failure
// Error carries a human-readable message.
type Response struct {
	Ok           bool           `json:"ok"`
	Error        string         `json:"error,omitempty"`
	PID          int            `json:"pid,omitempty"`
	Uptime       string         `json:"uptime,omitempty"`
	Config       string         `json:"config,omitempty"`
	Devices      []string       `json:"devices,omitempty"`
	Layout       int            `json:"layout"` // current XKB group; 0 is a meaningful value
	WordBuffer   string         `json:"word_buffer,omitempty"`
	ActiveWindow string         `json:"active_window,omitempty"`
	Hotkeys      int            `json:"hotkeys,omitempty"`       // number of [[Hotkeys]] rules in effect
	AutoReplaces int            `json:"auto_replaces,omitempty"` // number of [[AutoReplace]] rules in effect
	AutoDetects  int            `json:"auto_detects,omitempty"`  // number of [AutoDetect] Languages entries in effect
	AutoDetected int            `json:"auto_detected,omitempty"` // words the daemon corrected since it started
	Data         map[string]any `json:"data,omitempty"`          // raw TOML content or parsed config for get_config
	Diary        string         `json:"diary,omitempty"`         // diary tail for get_diary
}
