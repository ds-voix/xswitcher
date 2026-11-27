package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSocketPathFollowsTheEnvBothHalvesRead pins the one rule that lets the GUI
// reach the daemon at all: both ask SocketPath(), so whatever the listener bound
// is what the dialer dials. The stand starts its daemon on a /tmp socket, and a
// GUI that ignored XSWITCHER_SOCK sat on the default path and reported a
// connection error while the daemon answered ping happily on its own.
func TestSocketPathFollowsTheEnvBothHalvesRead(t *testing.T) {
	// t.Setenv only registers the restore; the case under test is the variable
	// being absent, which is how an installed daemon and GUI run.
	t.Setenv("XSWITCHER_SOCK", "")
	if err := os.Unsetenv("XSWITCHER_SOCK"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if got := SocketPath(); got != DefaultSocketPath {
		t.Errorf("without XSWITCHER_SOCK: got %q, want %q", got, DefaultSocketPath)
	}
	// An empty value is passed through, not replaced: the daemon then fails to
	// bind and says so, which beats listening somewhere nobody asked for.
	for _, want := range []string{"/tmp/gui.sock", ""} {
		t.Setenv("XSWITCHER_SOCK", want)
		if got := SocketPath(); got != want {
			t.Errorf("XSWITCHER_SOCK=%q: got %q", want, got)
		}
	}
}

// End-to-end over a real unix socket: the client methods added in Phase 0
// must round-trip through the server dispatch and the registered handlers.
func TestConfigCommandsRoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "test.sock")

	var setCalls []map[string]any
	store := map[string]any{"ScanDevices": map[string]any{"Test": "/dev/input/event1"}}
	validateErr := error(nil)

	srv := NewServer(sock,
		func() Response { return Response{Ok: true, PID: 4242} },
		func() Response { return Response{Ok: true} },
	)
	srv.SetConfigHandler(
		func() (map[string]any, error) { return store, nil },
		func(mods map[string]any) error {
			setCalls = append(setCalls, mods)
			return nil
		},
		func() error { return validateErr },
	)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c := NewClient(sock)

	if err := c.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	resp, err := c.Status()
	if err != nil || !resp.Ok || resp.PID != 4242 {
		t.Fatalf("status: resp=%+v err=%v", resp, err)
	}

	data, err := c.GetConfig()
	if err != nil {
		t.Fatalf("get_config: %v", err)
	}
	scan, ok := data["ScanDevices"].(map[string]any)
	if !ok || scan["Test"] != "/dev/input/event1" {
		t.Fatalf("get_config payload: %+v", data)
	}

	mods := map[string]any{"Keyboard.Delay": float64(9)}
	if err := c.SetConfig(mods); err != nil {
		t.Fatalf("set_config: %v", err)
	}
	if len(setCalls) != 1 || setCalls[0]["Keyboard.Delay"] != float64(9) {
		t.Fatalf("set_config handler saw: %+v", setCalls)
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	validateErr = errFakeValidate
	if err := c.Validate(); err == nil {
		t.Fatal("validate must surface the daemon error")
	}
}

type fakeError string

func (e fakeError) Error() string { return string(e) }

const errFakeValidate = fakeError("config invalid")
