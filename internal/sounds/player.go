// Package sounds plays event audio on the user's side. The daemon runs as
// root and must not touch the session sound server, so it only emits
// events over IPC and the GUI process plays the configured file through a
// system CLI player.
package sounds

import "os/exec"

// Player plays wav files fire-and-forget through the first available
// system player. A missing player degrades to silence, never to errors:
// sound is feedback, not function.
type Player struct {
	bin  string
	args []string
}

var candidates = [][]string{
	{"paplay"},      // PulseAudio
	{"pw-play"},     // PipeWire
	{"aplay", "-q"}, // plain ALSA, quiet
}

// New picks the player binary once at startup.
func New() *Player {
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			return &Player{bin: c[0], args: c[1:]}
		}
	}
	return &Player{}
}

// Play starts one background playback process and returns immediately.
func (p *Player) Play(path string) {
	if p.bin == "" || path == "" {
		return
	}
	cmd := exec.Command(p.bin, append(append([]string{}, p.args...), path)...)
	if err := cmd.Start(); err != nil {
		return
	}
	go cmd.Wait() // reap the child; its exit status is nobody's business
}
