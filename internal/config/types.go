// Package config holds the configuration types and parsing logic shared between
// the xswitcher daemon (cgo/X11) and the GUI (Fyne, no cgo). Only pure-Go types
// live here; compiled regexps and other runtime state stay in src/xswitcher.go.
package config

import "regexp"

// TScanDevices describes which evdev nodes to monitor.
type TScanDevices struct {
	Test    string `default:"/dev/input/event0"`
	Respawn int    `default:"30"`
	Search  string `default:"/dev/input/event*"`
	Bypass  string `default:"(?i)Video|Camera"`
	// BypassRE is compiled at runtime from Bypass; it is not read from TOML.
	BypassRE *regexp.Regexp `toml:"-"`
}

// TKeyboard controls virtual-keyboard timing.
type TKeyboard struct {
	Delay           int `default:"5"`
	WipeBySelection bool
}

// TActionKeys lists the key names that trigger layout switching and word actions.
type TActionKeys struct {
	Layouts   []int `default:"[0,1]"`
	Add       []string
	Drop      []string
	Test      []string
	StateKeys []string
}

// TWindowClass maps a window-class regex to an action chain.
type TWindowClass struct {
	Regex           string
	MouseClickDrops bool
	Actions         string
	// RE is compiled at runtime from Regex; it is not read from TOML.
	RE *regexp.Regexp `toml:"-"`
}

// TActions holds the global action parameters plus custom action chains.
type TActions struct {
	SeqLength     int    `default:"8"`
	WordChars     string `default:"(^([0-9A-Z=-]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9]):0$)"`
	WordHead      string `default:"(^([0-9A-Z]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9]):1$)"`
	NewWord       []string
	NewSentence   []string
	Compose       []string
	TypeClipboard []string
	Custom        map[string][]string // >> TAction.Name
}

// TAction defines a single named action (layout switch, exec, chain, etc.).
type TAction struct {
	Action      []string
	Layout      int `default:"-1"`
	Layouts     []int
	Exec        string
	Timeout     int    `default:"100"`
	Wait        bool   `default:"false"`
	SendBuffer  string `default:"WORD"`
	UseShell    bool   `default:"true"`
	Directory   string
	CleanEnv    bool `default:"true"`
	Environment []string
	UID         string
	GID         string
}

// TWayland carries Wayland-specific bypass and layout keys.
type TWayland struct {
	BypassX bool `default:"false"`
	Layout0 []string
	Layout1 []string
	Layout2 []string
	Layout3 []string
	Delay   int `default:"50"`
}

// TSequence is a compiled rule sequence (runtime only, not serialised).
type TSequence struct {
	OFF            *regexp.Regexp
	ON             *regexp.Regexp
	SEQ            *regexp.Regexp
	TailUnprovable bool
}

// TSequences is a slice of compiled rule sequences.
type TSequences []TSequence
