package main
/*
 xswitcher v1.0 pre-release
 Fully customizable low-level keyboard helper for X.Org-based linux desktop.
/////////////////////////////////////////////////////////////////////////////
 Copyright (C) 2020-2021 Dmitry Svyatogorov ds@vo-ix.ru
    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU Affero General Public License as
    published by the Free Software Foundation, either version 3 of the
    License, or (at your option) any later version.
    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU Affero General Public License for more details.
    You should have received a copy of the GNU Affero General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.
/////////////////////////////////////////////////////////////////////////////

  This soft logs the last keyboard events from Xorg-based linux desktop and does custom actions 
according to regex-based rules. The basic actions are to switch the language (keyboard layout)
in the window, and to retype the word in new layout.
  Any actions can be joined into chain, to make the complex things.

  There is also an "Exec" action, to run any external executables.

  Look "xswitcher.conf" config file for details.

!!! This is in fact the low-level keylogger together with the virtual keyboard. !!!
(Big thanks to "github.com/gvalkov/golang-evdev" "github.com/micmonay/keybd_event")

  So, it must have the root privileges by design. And it can't to be configured on per-user basis.
(But anybody is free to fork this project and implement any extra functionality.)

Referrers:
 https://www.kernel.org/doc/html/latest/input/event-codes.html
 https://www.kernel.org/doc/html/latest/input/uinput.html

 https://janczer.github.io/work-with-dev-input/
 https://godoc.org/github.com/gvalkov/golang-evdev#example-Open
 https://github.com/ds-voix/VX-PBX/blob/master/x%20switcher/draft.txt

 https://github.com/BurntSushi/xgb/blob/master/examples/get-active-window/main.go

 xgb is dumb overkill. To be replaced.
 X11 XGetInputFocus() etc. HowTo:
 https://gist.github.com/kui/2622504
*/

/*
 #cgo LDFLAGS: -lX11
 #include "C/x11.c"
*/
import "C"

import (
	"xswitcher/embeddedConfig"
	"xswitcher/exec"
	cfg "xswitcher/internal/config"
	"xswitcher/internal/ipc"
	flag "github.com/spf13/pflag"       // CLI keys like python's "argparse". More flexible, comparing with "gnuflag"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
//	"unsafe"
//	"github.com/gvalkov/golang-evdev"   // Keyboard and mouse events
	// Refactored to holoplot/go-evdev:
	"github.com/holoplot/go-evdev"      // Go support for the Linux evdev interface
	"github.com/micmonay/keybd_event"   // Virtual keyboard !!(must be improved to deal with complex input)
	"github.com/kballard/go-shellquote" // joining/splitting strings using sh's word-splitting rules
	"github.com/fsnotify/fsnotify"      // inotify on /dev/input/
	// Deal with clipboard
	"golang.design/x/clipboard"
)

// Config types are defined in internal/config/types.go so the GUI binary
// can import them without pulling in cgo/X11/evdev dependencies.
// Type aliases keep existing code in this file compiling unchanged.
type TScanDevices = cfg.TScanDevices
type TKeyboard = cfg.TKeyboard
type TActionKeys = cfg.TActionKeys
type TWindowClass = cfg.TWindowClass
type TActions = cfg.TActions
type TAction = cfg.TAction
type TWayland = cfg.TWayland
type TSequence = cfg.TSequence
type TSequences = cfg.TSequences
type TAutoReplace = cfg.TAutoReplace
type THotkey = cfg.THotkey
type TDiary = cfg.TDiary
type TAutoDetect = cfg.TAutoDetect

// Scan-codes
type t_key struct {
	code uint16;
	value int32; // 1=press 2=repeat 0=release
}

type t_keys []t_key

//type keyFunc func(event t_key)
type actionFunc func(*TAction)


const (
	DAEMON_NAME = "xswitcher"
	KEYS_SIZE = 768
)

var (
	CONFIG_PATH string = "/etc/xswitcher/xswitcher.conf"

	// daemonStartTime anchors the "uptime" field of the IPC status response.
	daemonStartTime = time.Now()

	debug bool
	DEBUG *bool = &debug

	verbose bool
	VERBOSE *bool = &verbose

	test_mode bool
	TEST_MODE *bool = &test_mode

	// Config
	ScanDevices TScanDevices
	Keyboard TKeyboard
	Templates map [string]string
	ActionKeys TActionKeys
	WindowClasses []TWindowClass
	Actions TActions
	ActionSet map[string] TAction
	Wayland TWayland
	AutoReplace []TAutoReplace
	Hotkeys []THotkey
	Diary TDiary
	AutoDetect TAutoDetect

	LANG = 0

	// Parse by regex
	ON *(regexp.Regexp) = regexp.MustCompile("(\\s|^)ON:\\([^\\s]+\\)") // "ON:(CTRL|ALT|META)"
	OFF *(regexp.Regexp) = regexp.MustCompile("(\\s|^)OFF:\\([^\\s]+\\)") // "OFF:(CTRL|ALT|META)"
	SEQ *(regexp.Regexp) = regexp.MustCompile("(\\s|^)SEQ:\\([^\\s]+\\)") // "SEQ:(@WORD@:2,@WORD@:0)"
	TEMPLATE *(regexp.Regexp) = regexp.MustCompile("@[A-Za-z0-9_]+@") // "@WORD@"

	Action = regexp.MustCompile("^Action\\..+")
	ActionName = regexp.MustCompile("\\..+$")
	// A "SEQ:" tail whose length is not fixed: bracket classes and "(?" group flags are
	// dropped first, so what is left of "* + ? { }" really is a repetition of a chain step.
	SEQ_BRACKET = regexp.MustCompile("\\[[^]]*\\]")
	SEQ_GROUP   = regexp.MustCompile("\\(\\?")
	SEQ_REPEAT  = regexp.MustCompile("[*+?{]")

	WordChars *(regexp.Regexp) // Actions.WordChars
	WordHead *(regexp.Regexp) // Actions.WordHead

	// Sequences
	NewWord TSequences
	NewSentence TSequences
	Compose TSequences
	TypeClipboard TSequences
	ActSeq map[string] TSequences

	// Scan-codes processing
	KEYS [KEYS_SIZE]func(t_key)
	ADD = make(map[uint16]bool)
	KEY_RANGE = regexp.MustCompile(`^[\[\]A-Z0-9_=.,;/\\-]+\.\.[\[\]A-Z0-9_=.,;/\\-]+$`)
	KEY_SPLIT = regexp.MustCompile(`\.\.`)

	// Action hooks
	ACTIONS = make(map[string] actionFunc, 5) // (RetypeWord, Switch, Layout, Respawn, Exec)

	// Shared t_key queues (it's ok to share *buffered* channels *writes*)
	keyboardEvents = make(chan t_key, 8)
	miceEvents = make(chan t_key, 8)

	// Node path -> a reader is running for it. An open fd survives the unlink of its node, so
	// re-creating /dev/input/eventN (a udev reload, a manual mknod) leaves the previous reader
	// alive on the same kernel device while the inotify CREATE starts another one next to it.
	// Both then push every keystroke into keyboardEvents and the state machine sees each key
	// twice. Measured: the duplicated stream desynchronised the PAUSE press/release counters
	// and RetypeWord leaked its own trigger into the output ("found pushed keys after retyping
	// was done!"). One reader per path is what this map guarantees.
	// A reused number is not a reason to skip a device: destroying a device wakes its blocked
	// read with an error, so the entry is gone before the watcher's next CREATE is handled (that
	// one sleeps a second per event). Measured: event0 came back for six consecutive keyboards,
	// and stand phase 13 (D5/D6) attaches the sixth one and switches with it.
	attached = make(map[string]bool)
	attachedMu sync.Mutex

	// Xorg via C-bindings
	display *C.struct__XDisplay
	revert_to C.int // Set 1-thread vars out of GC
	x_class *C.XClassHint
	// Cache ActiveWindowId() along key processing
	ActiveWindowId C.Window
	ActiveWindowClass string
	WC *TWindowClass

	// Virtual keyboard
	kb keybd_event.KeyBonding

	// Buffers
	WORD t_keys // addKey
	SENTENCE t_keys // addKey
	TEST t_keys // addKey + testKey
	REPEATED [KEYS_SIZE]bool // Deduplicate repeated codes (code:1,code:2,..code:2,code:0). There could be more then 1 "sealing" key.
	// A Builder is used to efficiently build a string using Write methods. It minimizes memory copying. The zero value is ready to use.
	TAIL strings.Builder // Text representation of TEST (the last Actions.SeqLength codes)
	CTRL = make(map[string]bool, 10) // A set of control keys that are down. Extra "WORD" indicates RetypeWord was just performed.

	CTRL_WORD map[string]bool // CTRL at the beginning of the WORD
	CTRL_SENTENCE map[string]bool // CTRL at the beginning of the SENTENCE
	COMPOSE int // Compose counter
	EXTRA int // Extra keys (to call the Action), must not be retyped.
	// The XKB group the last WORD character was typed in, or -1 while no character was typed since
	// the buffer was cleaned. RetypeWord replays raw scancodes and the server translates them with the
	// group active at the replay, so this is the only record of which layout the text the application
	// holds really came from (issue #14).
	wordLayout = -1
	DOWN = make(map[uint16] int) // In any case, all virtual keys MUST BE RELEASED at the end of retyping.

	clipboardOk = false
)

// A WORD typed in one window must not be lost when the focus briefly goes to another window and
// comes back (issue #13: Opera -> yakuake -> Opera between two letters). The global buffer
// variables above stay the "active" buffer for the window that currently owns input focus; on a
// focus change the outgoing buffer is saved under its window id and the incoming window's buffer
// is restored (or started empty). The virtual "WORD" state key is owned by the buffer, not by the
// machine-wide modifier set, so it travels with it via wordDone, and so does the layout the word was
// typed in (wordLayout): a different window can have its own idea of "the layout I am on".
type winBuffers struct {
	TEST, WORD, SENTENCE t_keys
	CTRL_WORD, CTRL_SENTENCE map[string]bool
	COMPOSE int
	wordDone bool // whether CTRL["WORD"] was set when this buffer was saved
	wordLayout int // the group the last character of this buffer was typed in (-1 = none yet)
}

var (
	winBuf    = map[C.Window]*winBuffers{}
	winLRU    []C.Window // most-recently-focused first, bounded by winBufMax
	bufWindow C.Window   // the window the active global buffers belong to
)

const winBufMax = 32 // LRU cap: a daemon running for months must not keep every closed window

func config() {
	var (
		conf_ []byte
		err   error
	)
	config_path := &CONFIG_PATH

	if env_config, ok := os.LookupEnv("CONFIG"); ok {
		*config_path = env_config
	}
	_, *DEBUG = os.LookupEnv("DEBUG")
	_, *VERBOSE = os.LookupEnv("VERBOSE")
	_, *TEST_MODE = os.LookupEnv("TEST")

	F := flag.NewFlagSet("", flag.ContinueOnError)
	config_path = F.StringP("conf", "c", *config_path, "Non-default config location")
	DEBUG = F.BoolP("debug", "d", *DEBUG, "Debug log level")
	VERBOSE = F.BoolP("verbose", "v", *VERBOSE, "Increase log level to NOTICE")
	TEST_MODE = F.BoolP("test", "t", *TEST_MODE, "Only output all key events to STDERR. No actions.")
	F.Init("", flag.ExitOnError)
	F.Parse(os.Args[1:])
	// Publish the effective path for the IPC config handlers below.
	CONFIG_PATH = *config_path

	conf_file, err := os.Open(*config_path)
	if err != nil {
		fmt.Println(fmt.Errorf("Config error: unable to open config file:\n%s", err.Error()))
		fmt.Println("* Using defaults!")
		conf_ = []byte(embeddedConfig.Toml)
	} else {
		defer conf_file.Close()
		conf_, err = ioutil.ReadAll(conf_file)
		if err != nil {
			fmt.Println(fmt.Errorf("Config error: unable to read config file:\n%s", err.Error()))
			fmt.Println("* Using defaults!")
			conf_ = []byte(embeddedConfig.Toml)
		}
	}

	if *DEBUG {
		fmt.Println(string(conf_))
	}

	parsed, err := cfg.ParseTOML(conf_)
	if err != nil {
		panic(fmt.Errorf("Config error: %s", err.Error()))
	}

	// Copy parsed config into daemon globals.
	ScanDevices = parsed.ScanDevices
	Keyboard = parsed.Keyboard
	Templates = parsed.Templates
	ActionKeys = parsed.ActionKeys
	WindowClasses = parsed.WindowClasses
	Actions = parsed.Actions
	ActionSet = parsed.ActionSet
	Wayland = parsed.Wayland
	AutoReplace = parsed.AutoReplace
	Hotkeys = parsed.Hotkeys
	Diary = parsed.Diary
	AutoDetect = parsed.AutoDetect

	collectManagedLayouts()
	buildAutoReplace()
	buildHotkeys()
	initDiary()
	buildAutoDetect()
}

// Substitute templates "@xxx@"
func template(str string) string {
	tpl := TEMPLATE.FindAllStringIndex(str, -1)
	if tpl != nil {
		for i := len(tpl)-1; i >= 0; i-- {
			match := tpl[i]
			t := str[ (match[0]+1):(match[1]-1) ]
			if _tpl, ok := Templates[t]; !ok {
				fmt.Printf("Parse warning: Template definition not found for @%s@. This expression will be used \"as is\".\n", t)
			} else {
				str = str[ 0:match[0] ] + _tpl + str[ (match[1]) : ]
			}
		}
	}
	return str
}

func seqParse(str string, act string) (seq TSequence) {
	var err error
	// 1. Substitute templates
	str = template(str)

	// 2. "OFF:()" state
	tpl := OFF.FindAllStringIndex(str, -1)
	if tpl != nil {
		if len(tpl) > 1 {
			panic(fmt.Errorf("Parse error: Found more than 1 \"OFF\" declaration for action \"%s\"", act))
		} else {
			match := tpl[0]
			t := strings.TrimLeft(str[ match[0] :match[1] ], " OFF:")
			seq.OFF, err = regexp.Compile(strings.TrimRight(t, "$") + "$") // Match againt the end of sequence
			if *VERBOSE || *DEBUG {
				fmt.Printf("%s OFF: %s\n", act, t)
			}
			if err != nil {
				panic(fmt.Errorf("Parse error: Invalid regex for OFF: condition of action \"%s\"\n%s", act, err.Error()))
			}
		}
	}
	// 3. "ON:()" state
	tpl = ON.FindAllStringIndex(str, -1)
	if tpl != nil {
		if len(tpl) > 1 {
			panic(fmt.Errorf("Parse error: Found more than 1 \"ON\" declaration for action \"%s\"", act))
		} else {
			match := tpl[0]
			t := strings.TrimLeft(str[ match[0] :match[1] ], " ON:")
			seq.ON, err = regexp.Compile(strings.TrimRight(t, "$") + "$") // Match againt the end of sequence
			if *VERBOSE || *DEBUG {
				fmt.Printf("%s ON: %s\n", act, t)
			}
			if err != nil {
				panic(fmt.Errorf("Parse error: Invalid regex for ON: condition of action \"%s\"\n%s", act, err.Error()))
			}
		}
	}
	// 4. "SEQ:()" state
	tpl = SEQ.FindAllStringIndex(str, -1)
	if tpl != nil {
		if len(tpl) > 1 {
			panic(fmt.Errorf("Parse error: Found more than 1 \"SEQ\" declaration for action \"%s\"", act))
		} else {
			match := tpl[0]
			t := strings.TrimLeft(str[ match[0] :match[1] ], " SEQ:")
			seq.SEQ, err = regexp.Compile(strings.TrimRight(t, "$") + "$") // Match againt the end of sequence
			if *VERBOSE || *DEBUG {
				fmt.Printf("%s => %s\n", act, t)
			}
			if err != nil {
				panic(fmt.Errorf("Parse error: Invalid regex for SEQ: condition of action \"%s\"\n%s", act, err.Error()))
			}
		}
	}

	return seq
}

// chainHasAction reports whether the action chain "name" ends in the built-in action
// "want", following the same "Action.xxx" references that doAction() expands.
func chainHasAction(name, want string) bool {
	seen := make(map[string]bool, len(ActionSet))
	var walk func(string) bool
	walk = func(n string) bool {
		if seen[n] { return false }
		seen[n] = true
		for _, act := range ActionSet[n].Action {
			if Action.MatchString(act) {
				if walk(strings.TrimLeft(ActionName.FindString(act), ".")) { return true }
			} else if act == want {
				return true
			}
		}
		return false
	}
	return walk(name)
}

// hasFreeQuantifier tells whether a compiled "SEQ:" tail can match a variable number of
// key events. RetypeWord() wipes len(WORD) - EXTRA keys and takes EXTRA from the length of
// the match (see testAction), so a tail of unfixed length silently changes how much is
// wiped. Measured on the stand: ".*PAUSE:1,PAUSE:0" matched the whole SeqLength window,
// EXTRA grew to 12 and RetypeWord did nothing at all --
// "RetypeWord error: WORD(12) is smaller than EXTRA(12)!" -- the layout switched, the word
// stayed in the wrong one. xswitcher.conf warns about it in a comment; say it at the start and
// never fire such a rule -- see TSequence.tailUnprovable.
func hasFreeQuantifier(pattern string) bool {
	// "[0-9A-Z=-]" and "[LR]_SHIFT" are character classes, not repetitions; "(?i)" and
	// "(?:" carry no length either. Nothing else may keep a "*", "+", "?" or "{".
	rest := SEQ_GROUP.ReplaceAllString(SEQ_BRACKET.ReplaceAllString(pattern, ""), "(")
	return SEQ_REPEAT.MatchString(rest)
}

func sequences() {
	var err error

	for _, s := range Actions.NewWord {
		NewWord = append(NewWord, seqParse(s, "NewWord"))
	}
	for _, s := range Actions.NewSentence {
		NewSentence = append(NewSentence, seqParse(s, "NewSentence"))
	}
	for _, s := range Actions.Compose {
		Compose = append(Compose, seqParse(s, "Compose"))
	}
	for _, s := range Actions.TypeClipboard {
		TypeClipboard = append(TypeClipboard, seqParse(s, "TypeClipboard"))
	}

	ActSeq = make(map[string] TSequences)
	for key, value := range Actions.Custom {
		retypes := chainHasAction(key, "RetypeWord")
		for _, s := range value {
			seq := seqParse(s, key)
			if retypes {
				rule := template(s)
				if tpl := SEQ.FindAllStringIndex(rule, -1); tpl != nil {
					t := strings.TrimLeft(rule[ tpl[0][0] :tpl[0][1] ], " SEQ:")
					if hasFreeQuantifier(t) {
						seq.TailUnprovable = true
						fmt.Printf("Parse warning: the rule for \"Action.%s\" ends in the RetypeWord action, but its SEQ tail \"%s\" matches a varying number of key events. RetypeWord takes the number of the shortcut's own events from that match, so it wipes too few characters or none at all. The rule will not fire; write the tail as the exact chain, e.g. \"SEQ:(PAUSE:1,PAUSE:0)\".\n", key, t)
					}
				}
			}
			ActSeq[key] = append(ActSeq[key], seq)
		}
	}

	WordChars, err = regexp.Compile(template(Actions.WordChars))
	if err != nil {
		panic(fmt.Errorf("Parse error: Invalid regex for Actions.WordChars\n%s", err.Error()))
	}
	if *VERBOSE || *DEBUG {
		fmt.Printf("WordChars => %s\n", template(Actions.WordChars))
	}

	WordHead, err = regexp.Compile(template(Actions.WordHead))
	if err != nil {
		panic(fmt.Errorf("Parse error: Invalid regex for Actions.WordHead\n%s", err.Error()))
	}
	if *VERBOSE || *DEBUG {
		fmt.Printf("WordHead => %s\n", template(Actions.WordHead))
	}

	return
}

func parseKeys(keys []string, action func(t_key), name string) {
	for _, key := range keys {
		if k, ok := key_def[key]; ok {
			KEYS[k] = action
		} else {
			if KEY_RANGE.MatchString(key) {
				k1 := uint(0)
				k2 := uint(0)
				kk := KEY_SPLIT.Split(key, 2)
				if k1, ok = key_def[ kk[0] ]; !ok {
					panic(fmt.Sprintf("Parse error: Invalid key for %s: %s", name, key))
				}
				if k2, ok = key_def[ kk[1] ]; !ok {
					panic(fmt.Sprintf("Parse error: Invalid key for %s: %s", name, key))
				}

				for k := k1; k <= k2 ; k++ {
					KEYS[k] = action
					if name == "Add" { // I found no way to take the valid pointer to func in go
						ADD[uint16(k)] = true
					}
				}
			} else {
				panic(fmt.Sprintf("Parse error: Invalid key for %s: %s", name, key))
			}
		}
	}
}

func keys() {
	for name, code := range key_def { // Invert key_def[] to key_name[]
		// The aliases below join key_def at the end of every call. An
		// inversion that read them back would give a second call two names
		// per code ("," and "COMMA") and let the map order pick which one
		// lands in key_name - the templates document that key_name must
		// carry the "canonic" spelling only. Skipping them keeps keys()
		// idempotent and the first call's result identical to before.
		if _, aliased := keyAliases[name]; aliased {
			continue
		}
		key_name[code] = name
	}
	for name, code := range keyAliases { // Extra key aliases
		key_def[name] = code
	}

	for i, _ := range KEYS { // All defaults to testKey()
		KEYS[i] = testKey
	}

	parseKeys(ActionKeys.Add, addKey, "Add")
	parseKeys(ActionKeys.Drop, dropKey, "Drop")
	parseKeys(ActionKeys.Test, testKey, "Test")

	return
}

// There must be 1 buffer per each X-window.
// Or just to reset the buffer on each focus change?
func getActiveWindowId() (idChanged bool) { // _Ctype_Window == uint32
	idChanged = true
	ActiveWindowId_old := ActiveWindowId
	focused := ActiveWindowId // Keep the previous value unless X names a real window
	if C.XGetInputFocus(display, &focused, &revert_to) == 0 {
		if *VERBOSE {
			fmt.Println("XGetInputFocus failed")
		}
		return false
	}
	if focused <= 1 { // None or PointerRoot
		// X goes through this state on every focus switch, and a key pressed while it
		// lasts is still the same user's word: dropping the buffers here wipes it.
		return false
	}
	ActiveWindowId = focused

	if ActiveWindowId == ActiveWindowId_old { return false}

	// !!! "X Error of failed request:  BadWindow (invalid Window parameter)" in case of window was gone.
	// https://eli.thegreenplace.net/2019/passing-callbacks-and-pointers-to-cgo/
	// https://artem.krylysov.com/blog/2017/04/13/handling-cpp-exceptions-in-go/
	// >>> https://stackoverflow.com/questions/32947511/cgo-cant-set-callback-in-c-struct-from-go
//	C.XFlush(display)
	if C.XGetClassHint(display, ActiveWindowId, x_class) > 0 { // "VirtualBox Machine"
		if ActiveWindowClass != C.GoString(x_class.res_name) {
			ActiveWindowClass = C.GoString(x_class.res_name)
			if *VERBOSE {
				fmt.Println("=", ActiveWindowClass)
			}
		}
	} else {
		if C.XGetClassHint(display, ActiveWindowId - 1, x_class) > 0 { // https://antofthy.gitlab.io/info/X/WindowID.txt
			// However the reported ID is generally wrong for GTK apps (like Firefox) and the windows immediate parent is actually needed...
			// Typically for GTK the parent window is 1 less than the focus window ... But there is no gurantee that the ID is one less.
			if ActiveWindowClass != C.GoString(x_class.res_name) {
				ActiveWindowClass = C.GoString(x_class.res_name)
				if *VERBOSE {
					fmt.Println("*", ActiveWindowClass)
				}
			}
		} else {
			if *DEBUG {
				fmt.Println("Empty ActiveWindowClass. M.b. gnome \"window\"?")
			}
		}
	}
	if C.xerror == C.True { // ??? Is there some action needed?
		C.xerror = C.False
	}
	return
}

func copyCTRL(c map[string]bool) (cp map[string]bool) {
	cp = make(map[string]bool, 8)
	for k, v := range c {
		cp[k] = v
	}
	return
}

func dropBuffers() {
	TEST = nil
	WORD = nil
	SENTENCE = nil
	delete(CTRL, "WORD")
	CTRL_WORD = copyCTRL(CTRL)
	CTRL_SENTENCE = copyCTRL(CTRL)
	COMPOSE = 0
	wordLayout = -1

	if *VERBOSE {
		fmt.Printf("dropBuffers()\n")
	}
}

func touchWindow(id C.Window) {
	for i, w := range winLRU {
		if w == id {
			winLRU = append(winLRU[:i], winLRU[i+1:]...)
			break
		}
	}
	winLRU = append([]C.Window{id}, winLRU...)
	for len(winLRU) > winBufMax {
		oldest := winLRU[len(winLRU)-1]
		winLRU = winLRU[:len(winLRU)-1]
		delete(winBuf, oldest)
	}
}

// saveBuffers snapshots the active global buffers under id, so that focusing back into id later
// can restore them. The "WORD" pseudo state key leaves the machine-wide CTRL map with the buffer.
func saveBuffers(id C.Window) {
	_, wd := CTRL["WORD"]
	winBuf[id] = &winBuffers{
		TEST: TEST, WORD: WORD, SENTENCE: SENTENCE,
		CTRL_WORD: CTRL_WORD, CTRL_SENTENCE: CTRL_SENTENCE,
		COMPOSE: COMPOSE, wordDone: wd, wordLayout: wordLayout,
	}
	delete(CTRL, "WORD")
	touchWindow(id)
}

// loadBuffers makes id the owner of the active global buffers: restore its snapshot if the window
// has been focused before, otherwise start from a clean buffer. Either way id becomes the current
// buffer window, and the LRU order is refreshed.
func loadBuffers(id C.Window) {
	b, ok := winBuf[id]
	if !ok {
		dropBuffers()
		touchWindow(id)
		return
	}
	TEST, WORD, SENTENCE = b.TEST, b.WORD, b.SENTENCE
	CTRL_WORD, CTRL_SENTENCE, COMPOSE = b.CTRL_WORD, b.CTRL_SENTENCE, b.COMPOSE
	wordLayout = b.wordLayout
	if b.wordDone {
		CTRL["WORD"] = true
	} else {
		delete(CTRL, "WORD")
	}
	touchWindow(id)
}

func newWord() {
	end := len(WORD) - 1
	if (end >= 0) && (WORD[end].value == 1) {
		WORD = WORD[end:]
	} else {
		WORD = nil
	}

	delete(CTRL, "WORD")
	CTRL_WORD = copyCTRL(CTRL)
	COMPOSE = 0
	wordLayout = -1 // whatever group the surviving pressed key came from is not recorded yet
}

func DropBuffers(A *TAction) {
	dropBuffers()
}

func Clean(A *TAction) {
	newWord()
}

// managedLayouts are the XKB groups this configuration asks xswitcher to handle: the global
// [ActionKeys] Layouts plus the lists and numbers of the actions that can really select a layout.
// The global list alone is not enough: Switch() walks the action's own Layouts and Layout() takes
// the action's own Layout, so a group reachable only through an action read as "extra language"
// here and every key of it was dropped -- the switch the config asked for landed in a layout where
// nothing is collected. A group named nowhere stays unmanaged, which is the "Don't impact extra
// languages (e.g., Chinese)" case xswitcher.conf describes.
var managedLayouts = map[int]bool{}

func collectManagedLayouts() {
	managedLayouts = make(map[int]bool, len(ActionKeys.Layouts))
	for _, l := range ActionKeys.Layouts {
		managedLayouts[l] = true
	}
	// Only a section that names the leaf itself may contribute. The `default:"-1"` tag does work --
	// measured: a [Action.X] section without a Layout parses as -1, because go-toml v1.9.5 applies
	// that tag itself (marshal.go, tagDefault) -- and the guard below skips it. The guard is still
	// what the code relies on, because a TAction built without the parser (a test, a future caller)
	// leaves the field at Go's zero 0 and would hand group 0 to xswitcher.
	for _, a := range ActionSet {
		for _, act := range a.Action {
			switch act {
			case "Switch":
				for _, l := range a.Layouts {
					managedLayouts[l] = true
				}
			case "Layout":
				if a.Layout >= 0 {
					managedLayouts[a.Layout] = true
				}
			}
		}
	}
}

// checkLanguageId reads the group the server is on and reports whether this configuration manages
// it. The group itself is returned too: checkAppend records it with the word, which is what tells a
// later retype which layout the characters were produced in (issue #14). One XkbGetState per key
// event serves both, so the snapshot costs no extra round trip. A read that fails reports -1, a
// group no list can name, so the caller treats the layout as unmanaged instead of silently claiming
// group 0 from an untouched struct.
func checkLanguageId() (int, bool) {
	state := new(C.struct__XkbStateRec)
	group := -1

	if rc := int(C.XkbGetState(display, C.XkbUseCoreKbd, state)); rc != 0 {
		if *VERBOSE || *DEBUG {
			fmt.Printf("checkLanguageId: XkbGetState returned %d\n", rc)
		}
	} else {
		group = int(state.group)
	}

	return group, managedLayouts[group]
}

func getXModifiers() uint32 {
	state := new(C.struct__XkbStateRec)
	C.XkbGetState(display, C.XkbUseCoreKbd, state);

	if (state.mods & 2) > 0 { // CAPSLOCK = 2
		CTRL[ key_name[evdev.KEY_CAPSLOCK] ] = true
	} else {
		delete(CTRL, key_name[evdev.KEY_CAPSLOCK])
	}

	if (state.mods & 16) > 0 { // NUMLOCK = 16
		CTRL[ key_name[evdev.KEY_NUMLOCK] ] = true
	} else {
		delete(CTRL, key_name[evdev.KEY_NUMLOCK])
	}

	return uint32(state.mods)
}

// Check language if (lang < 0), set language if (lang >= 0)
func Language(lang int) (int) {
	if Wayland.BypassX { // The crunch for KDE@Wayland
		if lang >= 0 {
			if LANG != lang {
				CTRL["WORD"] = true
				switch lang {
				case 0:
					sendKeySequence(Wayland.Layout0)
				case 1:
					sendKeySequence(Wayland.Layout1)
				case 2:
					sendKeySequence(Wayland.Layout2)
				case 3:
					sendKeySequence(Wayland.Layout3)
				}
				time.Sleep(time.Duration(Wayland.Delay) * time.Millisecond)
				sendSound("layout_switch")
			}
			LANG = lang
		}
		if *VERBOSE {
			fmt.Printf("Language(Wayland): %v >> %v\n", lang, LANG)
		}
		return LANG
	}

	state := new(C.struct__XkbStateRec)
	layout := C.uint(0)

	if rc := int(C.XkbGetState(display, C.XkbUseCoreKbd, state)); rc != 0 && (*VERBOSE || *DEBUG) {
		fmt.Printf("Language: XkbGetState returned %d\n", rc)
	}
	if lang >= 0 {
		if int(state.group) != lang {
			CTRL["WORD"] = true
		}
		wanted := int(state.group) != lang // the switch intent, before the lock rewrites state
		layout = C.uint(lang)
		// XkbLockGroup returns a Status, and its meaning here is not established. Measured on the
		// stand: locking group 0 answered 1 in runs where XkbGetState right after read group 0 back -
		// a request that plainly took effect - while locking group 1 answered 0. So a non-zero answer
		// is not "the switch failed" and must not be reported as that. Which group is in effect is
		// what the re-read below says; this line only surfaces the answer the call gave.
		if rc := int(C.XkbLockGroup(display, C.XkbUseCoreKbd, layout)); rc != 0 && (*VERBOSE || *DEBUG) {
			fmt.Printf("Language: XkbLockGroup(%d) answered %d\n", lang, rc)
		}
		if rc := int(C.XkbGetState(display, C.XkbUseCoreKbd, state)); rc != 0 && (*VERBOSE || *DEBUG) {
			fmt.Printf("Language: XkbGetState(after) returned %d\n", rc)
		}
		if wanted && int(state.group) == lang { // the group really moved — feedback time
			sendSound("layout_switch")
		}
	}

	if *VERBOSE {
		fmt.Printf("Language: %v >> %v\n", lang, int(state.group))
	}
	return int(state.group)
}

// Push or release the key on virtual keyboard
func sendKey(key t_key) {
	var err error
	switch key.value {
	case 0:
		err = kb.Up(key.code)
		if e := kb.Sync(); err == nil {
			err = e
		}
	default:
		err = kb.Down(key.code)
		if e := kb.Sync(); err == nil {
			err = e
		}
	}
	// kb.Up/Down/Sync write to the uinput device and can fail (device closed, ENODEV). A failed
	// write during a retype burst is silent otherwise, so surface it - under DEBUG only, because a
	// dead device would otherwise flood stdout once per key.
	if err != nil && *DEBUG {
		fmt.Printf("sendKey(%d=%d) failed: %v\n", key.code, key.value, err)
	}
	time.Sleep(time.Duration(Keyboard.Delay) * time.Millisecond)
}

// Single key press on virtual keyboard
func pressKey(key int) {
	sendKey(t_key{uint16(key), 1})
	sendKey(t_key{uint16(key), 0})
}

func sendKeySequence(keys []string) {
	if *VERBOSE {
		fmt.Printf("sendKeySequence: %v\n", keys)
	}
	for i := 0; i < len(keys); i++ {
		c := strings.Split(keys[i], ":")
		if len(c) > 1 {
			state, err := strconv.Atoi(c[1])
			if err != nil {
				continue
			}
			if (state < 0) || (state > 2) {
				continue
			}
			sendKey(t_key{uint16(key_def[c[0]]), int32(state)})
		}
	}

}

// ACTIONS (RetypeWord, Switch, Layout, Respawn, Exec)
func RetypeWord(A *TAction) {
	if (len(WORD) - EXTRA) < 1 { // WTF?!
		fmt.Printf("RetypeWord error: WORD(%v) is smaller than EXTRA(%v)!\n", len(WORD), EXTRA)
		sendSound("error")
		newWord()
		return
	}
	count := 0 // Count chars to be deleted
	seq := 0 // Incremental key event counter

	// Issue #14: the replay below sends raw scancodes, and the server translates them with the layout
	// that is active AT THE REPLAY. The action chain runs Switch() first, and Switch() steps on from
	// the group it reads at that moment - so when the layout changed without xswitcher seeing a word
	// character (another program, a desktop shortcut, an application doing it itself) the step can
	// arrive back on the very group the word was typed in, and the "corrected" word comes out
	// identical to the broken one. If it did, lock the group the word was NOT typed in.
	// Wayland stays out of this: under [Wayland] BypassX the daemon cannot observe a layout change it
	// did not make itself, Language() there only reports its own cache, and correcting from that cache
	// would double the switch the desktop already performed.
	if !Wayland.BypassX && wordLayout >= 0 && len(ActionKeys.Layouts) > 0 {
		if active := Language(-1); active == wordLayout {
			Language(nextLayout(wordLayout, ActionKeys.Layouts))
		}
	}

	// Patch "orphaned" key releases.
	o := make(map[uint16] bool) // True since key-down till key-up.
	for i := 0; i < (len(WORD) - EXTRA); i++ {
		if WORD[i].value == 0 {
			if !o[WORD[i].code] {
				continue
			} else {
				o[WORD[i].code] = false
			}
		} else {
			o[WORD[i].code] = true
		}

		if WordChars.MatchString(key_name[WORD[i].code] + ":" + strconv.Itoa(int(WORD[i].value))) {
			count++
		}
		if WORD[i].value == 0 {
			switch WORD[i].code {
			case uint16(key_def["SPACE"]):
				count++
			case uint16(key_def["BACKSPACE"]):
				fmt.Println("RetypeWord error: BACKSPACE inside WORD!")
				newWord()
				return
			}
		}
	}

	// Clean the word
	wipe := count - COMPOSE
	if *VERBOSE {
		fmt.Printf("BACKSPACE: %v - %v = %v\n", count, COMPOSE, wipe)
	}
	if Keyboard.WipeBySelection && wipe > 0 {
		// P1-6: the burst below asks the application for `wipe` separate edits, one per
		// Keyboard.Delay, and an editor that coalesces or loses any of them keeps part of the word -
		// the replay then duplicates that remainder. Selecting the characters and deleting the
		// selection is one edit, and typing over a selection is what every text widget already does
		// on the first BackSpace, so a dropped keystroke shortens a selection instead of leaving a
		// half-deleted word behind.
		sendKey(t_key{uint16(key_def["L_SHIFT"]), 1})
		for i := 0; i < wipe; i++ {
			pressKey(evdev.KEY_LEFT)
		}
		sendKey(t_key{uint16(key_def["L_SHIFT"]), 0})
		pressKey(evdev.KEY_BACKSPACE)
	} else {
		for i := 0; i < wipe; i++ {
			pressKey(evdev.KEY_BACKSPACE)
		}
	}

	// Initial CTRL state
	for k, v := range CTRL_WORD {
		if v && ! CTRL[k] {
			switch key_def[k] {
				case evdev.KEY_CAPSLOCK:
					pressKey(evdev.KEY_CAPSLOCK)
				case evdev.KEY_NUMLOCK:
					pressKey(evdev.KEY_NUMLOCK)
				default:
					sendKey(t_key{uint16(key_def[k]), 1})
					DOWN[uint16(key_def[k])] = seq
					seq++
			}
		}
	}

	// Retype WORD
	RETYPE := len(WORD) - EXTRA

	if *VERBOSE {
		fmt.Printf("RETYPE: %v >> %v", RETYPE, CTRL_WORD) // Helpfull to debug e.g. keyboard bounce
		// "RETYPE: {17 1}{16 0}{17 0} :DONE" >> Oh, shi!
	}

	for i := 0; i < RETYPE; i++ {
		if WORD[i].value == 0 {
			if !o[WORD[i].code] {
				switch WORD[i].code { // !! There must be StateKeys-driven check
				case evdev.KEY_LEFTCTRL, evdev.KEY_LEFTSHIFT, evdev.KEY_LEFTALT, evdev.KEY_RIGHTCTRL, evdev.KEY_RIGHTSHIFT, evdev.KEY_RIGHTALT, evdev.KEY_LEFTMETA, evdev.KEY_RIGHTMETA:
				default:
					if *VERBOSE {
						fmt.Printf("{%d x}", WORD[i].code)
					}
					continue
				}
			} else {
				o[WORD[i].code] = false
			}
		} else {
			o[WORD[i].code] = true
		}
		sendKey(WORD[i])
		if WORD[i].value == 1 {
			DOWN[WORD[i].code] = seq
		} else {
			delete(DOWN, WORD[i].code)
		}
		seq++
		if *VERBOSE {
			fmt.Printf("%v",WORD[i])
		}
	}
	if *VERBOSE {
		fmt.Printf(" :DONE\n")
	}
	WORD = WORD[ 0 : (len(WORD) - EXTRA)]
	CTRL["WORD"] = true
	sendSound("retype")
	// Whatever the application holds now is the replayed text, produced in the layout that is active
	// at this moment - so the snapshot follows the word instead of staying on the layout it came from.
	wordLayout = Language(-1)

	// Clear virtual keyboard state
	if len(DOWN) > 0 {
		fmt.Printf("RetypeWord warning: found pushed keys after retyping was done! %v", DOWN)
		for k, _ := range DOWN {
			sendKey(t_key{k, 0})
		}
	}
}

// nextLayout returns the layout that follows ref inside the cyclic list. Switch() stepped with
// `next = l + 1`, which is the group VALUE used later as a list INDEX, so only a list starting at
// group 0 cycled: with Layouts = [1, 2] and the server on group 1 it computed next = 2, wrapped it
// by `next >= len(Layouts)` and re-selected Layouts[0] = 1 -- the group it had just left. Walking
// positions keeps the shipped [0, 1] answers identical and makes [1, 2] cycle 1 >> 2 >> 1.
func nextLayout(ref int, layouts []int) int {
	for i, l := range layouts {
		if l == ref {
			return layouts[(i + 1) % len(layouts)]
		}
	}
	return layouts[0] // a group the list does not name: start from its beginning, as before
}

// ToDo: newWord() | dropBuffers() must be implemented here in "smart" way.
// Nested action: leave buffers as is. Or implement extra option "what to do with buffers".
// Single action: drop. Or be smarter and remember the layout inside key sequences...
func Switch(A *TAction) {
	if len(A.Layouts) == 0 { // A.Layouts[0] below would panic and the daemon would die on this key
		fmt.Printf("Switch warning: the action lists no Layouts to cycle through\n")
		return
	}

	Language(nextLayout(Language(-1), A.Layouts))

	CTRL["WORD"] = true
	noteManualSwitch()
}

func Layout(A *TAction) {
	Language(A.Layout)
	noteManualSwitch()
}

// resolveRunAs turns the [Action.X] "run_as" config (UID/GID) into the numeric credentials that
// exec.ExecCommand applies. exec.ExecCommand only sets SysProcAttr.Credential when UID > 0, so
// (0, 0) means "run as the current user" - the documented default when a config omits UID. The old
// code called user.Lookup(A.UID) unconditionally, and looking up the empty string fails, so Exec
// returned before starting the command: every [Action.X] Exec without a UID was a silent no-op.
func resolveRunAs(uid, gid string) (uint32, uint32, error) {
	if len(uid) == 0 {
		return 0, 0, nil
	}
	user_, err := user.Lookup(uid)
	if err != nil {
		return 0, 0, err
	}
	u64, err := strconv.ParseUint(user_.Uid, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("non-integer uid for user %q: %v", uid, err)
	}

	gid_ := user_.Gid
	if len(gid) > 0 {
		group, err := user.LookupGroup(gid)
		if err != nil {
			return 0, 0, err
		}
		gid_ = group.Gid
	}
	g64, err := strconv.ParseUint(gid_, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("non-integer gid for user %q: %v", uid, err)
	}
	return uint32(u64), uint32(g64), nil
}

// The glyphs the canonical key names of src/keys.go produce on a US layout: index 0 is the
// unshifted glyph, index 1 the shifted one, and a one-character value means the key gives the same
// character either way. Letters need no entry because their name already is the shifted glyph, and
// neither do the digits, whose names "1".."0" are the unshifted glyphs.
var usGlyphs = map[string]string{
	"-": "-_", "=": "=+", "GRAVE": "`~", "APOSTROPHE": "'\"", "SEMICOLON": ";:",
	"L_BRACE": "[{", "R_BRACE": "]}", "BACKSLASH": "\\|", "COMMA": ",<", "DOT": ".>", "SLASH": "/?",
	"KP0": "0", "KP1": "1", "KP2": "2", "KP3": "3", "KP4": "4",
	"KP5": "5", "KP6": "6", "KP7": "7", "KP8": "8", "KP9": "9",
}

// glyphFunc spells one key: the canonical name of the position (as key_name
// records it) and the Shift/CapsLock state in effect, and it returns the
// character that key produces in its layout, or false for a key that produces
// none. The evdev codes an event carries name a key POSITION, never a glyph, so
// every layout this can spell is one whose position table the daemon ships.
type glyphFunc func(name string, shift, caps bool) (rune, bool)

// usGlyph is the US row of those tables: the layout whose position numbering
// src/keys.go and the shipped [ActionKeys] Layouts = [0, 1] are written against.
// A word typed in another group comes out as the latin glyphs of the same
// physical keys, which is what a replay of the buffer would do. Spelling a group
// from the server's own XKB map would be a change of its own; the one other
// script the daemon ships is ruGlyph in src/autodetect.go.
func usGlyph(name string, shift, caps bool) (rune, bool) {
	if glyph, ok := usGlyphs[name]; ok {
		r := rune(glyph[0])
		if shift && len(glyph) > 1 {
			r = rune(glyph[1])
		}
		return r, true
	}
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		// A letter is uppercase when exactly one of Shift and CapsLock is in effect.
		if shift != caps {
			return rune(name[0]), true
		}
		return rune(name[0] - 'A' + 'a'), true
	}
	if len(name) == 1 && name[0] >= '0' && name[0] <= '9' { // The names "1".."9" and "0".
		if shift {
			return rune(")!@#$%^&*("[name[0]-'0']), true
		}
		return rune(name[0]), true
	}
	// Anything else (function keys, modifiers, navigation) produced no character.
	return 0, false
}

// renderBuffer walks the events a buffer collected and spells them with the given layout table:
// bufferToText below is the latin one, bufferToRu the Cyrillic one.
//
// CapsLock starts from the state the daemon keeps for the machine (getXModifiers() refreshes
// CTRL from Xkb after every event) and then applies the CAPS presses inside the buffer, so a buffer
// that toggled it mid-word still renders the way it was typed.
func renderBuffer(buf t_keys, glyph glyphFunc) string {
	shift := false
	caps := CTRL["CAPS"]
	text := make([]rune, 0, len(buf))
	for _, k := range buf {
		name := key_name[k.code]
		switch name {
		case "L_SHIFT", "R_SHIFT":
			shift = k.value > 0
			continue
		case "CAPS":
			if k.value == 1 {
				caps = !caps
			}
			continue
		case "BACKSPACE":
			if k.value == 1 && len(text) > 0 {
				text = text[ : len(text)-1]
			}
			continue
		case "TAB":
			if k.value == 1 {
				text = append(text, '\t')
			}
			continue
		case "ENTER":
			if k.value == 1 {
				text = append(text, '\n')
			}
			continue
		case "SPACE":
			if k.value == 1 {
				text = append(text, ' ')
			}
			continue
		}
		if k.value != 1 { // Only a press produces a character; its release and any repeat do not.
			continue
		}
		if r, ok := glyph(name, shift, caps); ok {
			text = append(text, r)
		}
	}
	return string(text)
}

// bufferToText renders the key events a buffer collected back into the characters they produced,
// so that an external command can be given the word instead of an internal dump.
func bufferToText(buf t_keys) string {
	return renderBuffer(buf, usGlyph)
}

func Exec(A *TAction) {
/*
	Exec string
	Timeout int       `default:"100"`
	Wait bool         `default:"false"`
	SendBuffer string `default:"WORD"`
	UseShell bool     `default:"true"`
	Directory string
	CleanEnv bool     `default:"true"`
	Environment []string 
	UID string
	GID string
*/
	var c exec.Command;
	args, err := shellquote.Split(A.Exec)
	if err != nil {
		fmt.Printf("Exec error: %s", err.Error())
		return
	}

	if len(args) == 0 {
		fmt.Println("Exec error: Empty \"Exec\" value!")
		return
	}

	c.UseShell = A.UseShell
	if A.UseShell {
		c.Command = A.Exec
	} else {
		c.Command = args[0]
		if len(args) > 1 {
			c.Args = args[1:]
		}
	}

	if A.SendBuffer != "" {
		switch A.SendBuffer {
		case "WORD":
			c.StdIn = []byte(bufferToText(WORD))
		case "SENTENCE":
			c.StdIn = []byte(bufferToText(SENTENCE))
		case "CLIPBOARD":
			// The text the user already selected/copied (issue #5). A word buffer cannot supply it:
			// xswitcher only sees keystrokes, while the selection lives in the X server. An unreadable
			// or non-text clipboard gives an empty stdin, the same as an empty WORD would.
			txt, _ := clipboardText()
			c.StdIn = []byte(txt)
		default:
			c.StdIn = []byte(A.SendBuffer)
		}
	}

	c.Timeout = float64(A.Timeout)
	c.No_wait = ! A.Wait
	c.Set_dir = A.Directory

	if ! A.CleanEnv {
		c.Set_env = os.Environ()
	}
	c.Set_env = append(c.Set_env, A.Environment...)

	// UID & GID: an empty UID keeps (0, 0) so exec runs as the current user (see resolveRunAs).
	c.UID, c.GID, err = resolveRunAs(A.UID, A.GID)
	if err != nil {
		fmt.Printf("Exec: run_as %q/%q invalid: %v\n", A.UID, A.GID, err)
		return
	}

	
	if *VERBOSE || *DEBUG {
		// The struct carries StdIn, so printing it whole would put the text a hook was handed -- and
		// since "CLIPBOARD" that text is whatever the user copied last -- into the log. Describe the
		// call and the size of its input instead; the modifier state is not user text and stays.
		fmt.Printf("Exec: %q args=%v shell=%v dir=%q timeout=%v wait=%v stdin=%dB ctrl=%v\n",
			c.Command, c.Args, c.UseShell, c.Set_dir, c.Timeout, !c.No_wait, len(c.StdIn), CTRL_WORD)
	}
	r := exec.ExecCommand(&c)

	if *VERBOSE || *DEBUG {
		fmt.Printf("%v = %v\n", string(r.StdOut), string(r.StdErr))
	}

}

// Perform actions depending on WindowClasses[]
func setWindowActions() {
	WC = nil
	// Depending on WindowClass
	for _, w := range WindowClasses {
		if w.RE != nil {
			if ActiveWindowClass == "" { // Empty class name match empty regex
				continue
			} else {
				if w.RE.MatchString(ActiveWindowClass) {
					WC = &w
					break
				}
			}
		} else {
			WC = &w
			break
		}
	}
	return
}

// sortedSeqNames returns the keys of an ActSeq map in a fixed (lexicographic) order. Range over a map
// yields keys in a randomized order, and the ActSeq firing loop has no break: several rules can match
// the same event and each doAction mutates the shared WORD/EXTRA/COMPOSE buffers, so an unordered
// range made the response to a given keystroke depend on Go's map seed. Sorting makes it reproducible.
func sortedSeqNames(seq map[string]TSequences) []string {
	names := make([]string, 0, len(seq))
	for name := range seq {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func testAction(t *TSequences) bool {
TEST:
	for _, test := range *t {
		if test.OFF != nil {
			for ctrl, state := range CTRL {
				if state && test.OFF.MatchString(ctrl) { continue TEST }
			}
		}
		if test.ON != nil {
			if len(CTRL) < 1 { continue TEST }
			on := false
			for ctrl, state := range CTRL {
				if state && test.ON.MatchString(ctrl) {
					on = true
					break
				}
			}
			if !on { continue TEST }
		}
		if test.SEQ != nil {
			if test.SEQ.MatchString(TAIL.String()[1:]) {
				if test.TailUnprovable {
					// The shortcut's own event count would be read off this match, and the match
					// has no fixed length. Firing would wipe a number nobody asked for, so let the
					// next rule of the same action try instead.
					continue TEST
				}
				tail := test.SEQ.FindString(TAIL.String()[1:]) // The last keys must be omited, e.g. while retyping word.
				// Count tail commas
				EXTRA = strings.Count(tail, ",") + 1
				skip := 0
				for i := 1; i <= EXTRA; i++ {  // Check that the extra keys are in ADD[] collection
					if !ADD[ TEST[len(TEST) - i].code ] {
						skip++ // Collected key is not an "EXTRA"
					}
				}
				EXTRA -= skip
				return true
			}
		}
	}
	return false
}

// ActionSet is the chain of (one ore more) TAction
func doAction(name *string) { // name of ActionSet
	a, ok := ActionSet[*name]
	if !ok { return } // Action not found?!

	for _, act := range a.Action {
		if Action.MatchString(act) { // Action.xxx >> recursive call
			if *DEBUG {
				fmt.Println(act, "[]")
			}
			name := strings.TrimLeft(ActionName.FindString(act), ".")
			doAction(&name)
		} else {
			if _, ok = ACTIONS[act]; !ok {
				fmt.Printf("WTF! No such action \"%s\"", act)
				return
			}
			if *DEBUG {
				fmt.Println(act)
			}
			ACTIONS[act](&a)
		}
	}

	return
}

func doWindowActions() {
	TAIL.Reset()
	// EXTRA is the event count the matched SEQ tail protects. testAction assigns it on every true
	// return and RetypeWord - the only reader - is reached through that return alone, so no path reads
	// a value an earlier event left behind. Clearing it with the rest of the per-event state keeps that
	// a property of this call rather than of the program's history: a consumer added later, or a rule
	// that stops carrying a tail, then wipes the word it actually holds.
	EXTRA = 0

	if WC == nil { return }
	if WC.Actions == "" { return }

	// Test sequence
	l := Actions.SeqLength
	if l > len(TEST) {
		l = len(TEST)
	}
	for i := len(TEST) - l ; i < len(TEST); i++ {
		TAIL.WriteString("," + key_name[TEST[i].code] + ":" + strconv.Itoa(int(TEST[i].value)))
	}
	if *DEBUG {
		fmt.Println(TAIL.String()[1:])
	}
	if testAction(&NewSentence) {
		if *VERBOSE || *DEBUG {
			fmt.Printf("NewSentence: %s\n", TAIL.String()[1:])
		}
		dropBuffers()
		return
	}
	if testAction(&Compose) {
		if *VERBOSE || *DEBUG {
			fmt.Printf("Compose: %s\n", TAIL.String()[1:])
		}
		COMPOSE++
		return
	}
	if testAction(&TypeClipboard) {
		if *VERBOSE || *DEBUG {
			fmt.Printf("TypeClipboard: %s\n", TAIL.String()[1:])
		}
		typeClipboard(nil)
		return
	}
	if testAction(&NewWord) {
		if *VERBOSE || *DEBUG {
			fmt.Printf("NewWord: %s\n", TAIL.String()[1:])
		}
		if EXTRA >= 0 && len(WORD)-EXTRA >= 1 { // the word the user just finished — diary time
			recordDiary(WORD[:len(WORD)-EXTRA])
		}
		tryAutoReplace()
		tryAutoDetect()
		newWord()
		return
	}
	for _, name := range sortedSeqNames(ActSeq) { // Is there some reason to do more then 1 action?
		act := ActSeq[name]
		if testAction(&act) {
			if *VERBOSE || *DEBUG {
				fmt.Printf("%s: %s\n", name, TAIL.String()[1:])
			}
			doAction(&name)
		}
	}
	return
}

func checkAppend(event t_key, slice ...*t_keys) {
	if event.value == 2 { // Repeated code
		if REPEATED[event.code] {
			return
		} else {
			REPEATED[event.code] = true
		}
	} else {
		REPEATED[event.code] = false
	}

	switch event.code {
	case evdev.KEY_LEFTCTRL, evdev.KEY_LEFTSHIFT, evdev.KEY_LEFTALT, evdev.KEY_RIGHTCTRL, evdev.KEY_RIGHTSHIFT, evdev.KEY_RIGHTALT, evdev.KEY_LEFTMETA, evdev.KEY_RIGHTMETA:
		if event.value > 0 {
			CTRL[ key_name[event.code] ] = true
		} else {
			delete(CTRL, key_name[event.code])
		}
		if *DEBUG {
			fmt.Println(CTRL)
		}
	}
	getXModifiers() // X can lag while setting NUMLOCK state (and m.b. CAPSLOCK too), so check it after each key event
	checkHotkeys(event) // CTRL now reflects this very event; combos work even on an unmanaged layout

	group, managed := checkLanguageId() // A group no action and the global [ActionKeys] Layouts list name:
	if ! managed {
		// drop the buffers, so switching back cannot retype a WORD left over from the last managed
		// group. dropBuffers() is idempotent, so calling it on every event while unmanaged is safe.
		dropBuffers()
		return
	}


	if getActiveWindowId() { // New focused window detected
		// Save the buffer the outgoing window had collected and restore the incoming window's own,
		// instead of dropping it: this is what keeps a half-typed word when focus blips to another
		// window and back (issue #13). A first-ever window starts from a clean buffer.
		if bufWindow > 1 { // skip None(0)/PointerRoot(1): not a real buffer owner
			saveBuffers(bufWindow)
		}
		loadBuffers(ActiveWindowId)
		bufWindow = ActiveWindowId
		setWindowActions()
	} else if WC == nil {
		// X named no window yet (None/PointerRoot, or no WM at all): without an action set
		// the daemon has nothing to run and the keyboard stays silently unswitched.
		setWindowActions()
	}

	for _, key := range slice {
		// Issue #14: remember the layout the characters of WORD were produced in. Only the word
		// buffer records it, and only for the events WordChars counts as characters to wipe, so the
		// trigger's own keys (PAUSE, the modifiers) never overwrite it. testKey() passes TEST alone,
		// so a mouse click cannot touch the snapshot either.
		if key == &WORD && WordChars.MatchString(key_name[event.code] + ":" + strconv.Itoa(int(event.value))) {
			wordLayout = group
		}
		*key = append(*key, event)
	}

	doWindowActions()
	return
}

func addKey(event t_key) {
	checkAppend(event, &TEST, &WORD, &SENTENCE)
	return
}

func testKey(event t_key) {
	checkAppend(event, &TEST)
	return
}

func dropKey(event t_key) {
	dropBuffers()
	return
}

func events(device *evdev.InputDevice) {
	path := device.Path()
	name, _ := device.Name()
	// Forget the path only when this reader really stops, so that a device attached again later
	// can be opened once more.
	defer func() {
		attachedMu.Lock()
		delete(attached, path)
		attachedMu.Unlock()
		updateDevicesSnapshot()
	}()
	for {
		event, err := device.ReadOne()
		if err != nil {
			fmt.Printf("Closing device \"%s\" due to an error:\n\"\"\" %s \"\"\"\n", name, err)
			return
		}

//		fmt.Printf("  K type %v, %v = %v\n", evdev.TypeName(event.Type), event.Code, event.Value)
		if event.Type == evdev.EV_KEY { // Key events
			if (event.Code >= evdev.BTN_LEFT) && (event.Code <= evdev.BTN_TASK) { // Mouse keys
				miceEvents <- t_key{uint16(event.Code), event.Value}
			} else {
				if key_name[uint16(event.Code)] != "" { // Don't collect unknown input
					keyboardEvents <- t_key{uint16(event.Code), event.Value}
				}
			}
		}
	}
}

func connectEvents(connectPath string) {
	devicePaths, err := evdev.ListDevicePaths()
	if err != nil {
		panic(fmt.Sprintf("Events error: Unable to list devices: %s", err))
//		return
	}
	for _, dev := range devicePaths {
		if (connectPath != "") && (dev.Path != connectPath) {
			continue
		}
		if dev.Name == keybd_event.DeviceName { // Own output would be read back as input
			if *VERBOSE {
				fmt.Printf("- %s:\t%s (own virtual keyboard)\n", dev.Path, dev.Name)
			}
			continue
		}
		attachedMu.Lock()
		reading := attached[dev.Path]
		attachedMu.Unlock()
		if reading { // A reader for this path is already running
			if *VERBOSE {
				fmt.Printf("- %s:\t%s (already attached)\n", dev.Path, dev.Name)
			}
			continue
		}
		skip := true
		d, err := evdev.Open(dev.Path)
		if err != nil {
			fmt.Printf("? %s:\t%s\n", dev.Path, dev.Name)
			fmt.Printf("Cannot read %s: %v\n", dev.Path, err)
			continue
		}
		if ScanDevices.BypassRE.MatchString(dev.Name) {
			// The descriptor was opened above and no reader takes it over, so close it here.
			// Left to itself it stays open until the garbage collector finalizes d: measured
			// with GOGC=off, three bypassed devices meant three held fds.
			_ = d.Close()
			if *VERBOSE {
				fmt.Printf("- %s:\t%s\n", dev.Path, dev.Name)
			}
			continue
		}

		for _, t := range d.CapableTypes() {
//			fmt.Printf("  Event type %d (%s)\n", t, evdev.TypeName(t))
			if t == evdev.EV_KEY {
				// Registered before the goroutine starts: events() removes the entry when it
				// leaves, so marking it later could let a second reader slip in.
				attachedMu.Lock()
				attached[dev.Path] = true
				attachedMu.Unlock()
				updateDevicesSnapshot()
				go events(d)
				skip = false
				if *VERBOSE {
					fmt.Printf("  %s:\t%s\n", dev.Path, dev.Name)
				}
				break
			}
		}
		if skip { // no EV_KEY capability: the reader never took d, so this is ours to close
			_ = d.Close()
			if *VERBOSE {
				fmt.Printf("x %s:\t%s\n", dev.Path, dev.Name)
			}
		}
	}
	return
}

// https://gravitational.com/blog/golang-ssh-bastion-graceful-restarts/
func forkChild() (*os.Process, error) {
	// Every respawn in this program funnels through here: the reload command, the Respawn
	// action a user binds to a hotkey or an [[Actions]] entry, and the re-exec that waits
	// for the DE at startup. Handing over a file the child cannot boot through kills the
	// keyboard through all three, so the states the shared parser can name are refused
	// once here instead of at each caller. configKillsChild says what still slips past it.
	if err := configKillsChild(); err != nil {
		return nil, err
	}
	// Pass stdin, stdout, and stderr to the child.
	files := []*os.File{
		os.Stdin,
		os.Stdout,
		os.Stderr,
	}

	// Get current process name and directory.
	execName, err := os.Executable()
	if err != nil {
		return nil, err
	}
	execDir := filepath.Dir(execName)

	// Spawn child process.
	p, err := os.StartProcess(execName, os.Args, &os.ProcAttr{
		Dir:   execDir,
		Env:   os.Environ(),
		Files: files,
		Sys:   &syscall.SysProcAttr{},
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func Respawn(*TAction) { // Completelly respawn xswitcher.
	p, err := forkChild()
	if err != nil {
		fmt.Printf("Respawn skipped: %v.\n", err)
		return
	}
	// A forked process is not a running one. Everything the parser cannot see -- the
	// expressions sequences() compiles, the keys keys() resolves, the device serve() stats
	// -- kills the child afterwards, and this process IS the keyboard: it may only let go
	// once the child has taken the socket. See handover.go.
	if err := handover(p); err != nil {
		fmt.Printf("Respawn skipped: %v.\n", err)
		return
	}
	fmt.Printf("Forked child %v.\n", p.Pid)
	os.Exit(0)
}

// --- IPC handlers for the GUI (see internal/ipc) -----------------------------

// The live status exposed to the GUI is split into two snapshots:
//
//   - coreSnapshot (layout, word buffer, active window, [AutoDetect] readings)
//     mirrors globals that only the serve() goroutine mutates, so it is only
//     ever written from that goroutine (and once at startup before serve()
//     begins).
//   - devicesSnapshot mirrors the `attached` map, which readers and the device
//     watcher mutate under attachedMu. It is refreshed on every attach/detach
//     so device changes are visible even while no key event wakes serve().
type statusCore struct {
	Lang   int
	Word   string
	Window string
	// WordGroup is the group the last word character was typed in (-1 before the
	// first one). [AutoDetect] keys its decision on this, not on Lang, so the GUI
	// preview needs it to answer about the same quantity the daemon judges.
	WordGroup int
	// Readings is the same buffer spelled in each [AutoDetect] language, so the
	// GUI can preview a verdict without owning the glyph tables. See
	// detectReadings for when it is filled.
	Readings map[string]string
}

var (
	coreSnapshot    atomic.Value // statusCore
	devicesSnapshot atomic.Value // []string
)

func updateDevicesSnapshot() {
	devices := make([]string, 0, len(attached))
	attachedMu.Lock()
	for path := range attached {
		devices = append(devices, path)
	}
	attachedMu.Unlock()
	sort.Strings(devices)
	devicesSnapshot.Store(devices)
}

func updateStatusSnapshot() {
	coreSnapshot.Store(statusCore{
		Lang:      LANG,
		Word:      bufferToText(WORD),
		Window:    ActiveWindowClass,
		WordGroup: wordLayout,
		Readings:  detectReadings(),
	})
	updateDevicesSnapshot()
}

func currentStatus() (statusCore, []string) {
	core, _ := coreSnapshot.Load().(statusCore)
	devices, _ := devicesSnapshot.Load().([]string)
	return core, devices
}

func ipcStatus() ipc.Response {
	core, devices := currentStatus()
	return ipc.Response{
		Ok:           true,
		PID:          os.Getpid(),
		Uptime:       time.Since(daemonStartTime).Truncate(time.Second).String(),
		Config:       CONFIG_PATH,
		Layout:       core.Lang,
		WordBuffer:   core.Word,
		ActiveWindow: core.Window,
		Readings:     core.Readings,
		WordGroup:    core.WordGroup,
		Devices:      devices,
		Hotkeys:      len(Hotkeys),
		AutoReplaces: len(AutoReplace),
		AutoDetects:  len(detectGroupOf),
		AutoDetected: int(atomic.LoadInt64(&autoDetectFixed)),
	}
}

func ipcReload() ipc.Response {
	// forkChild() refuses to hand over a config the child could not parse, and a refusal
	// means this process and its file both stay exactly as they were -- which is what the
	// GUI has to hear, since it asked for a reload that did not happen.
	p, err := forkChild()
	if err != nil {
		return ipc.Response{Ok: false, Error: fmt.Sprintf("reload refused, config kept as is: %v", err)}
	}
	// forkChild() only created a process. Until it owns the socket it has not passed the
	// states the parser cannot see, and answering Ok before that is the lie that costs the
	// user their keyboard: this process exits, the child never comes up, and the GUI is
	// left reporting a reload that never happened.
	if err := handover(p); err != nil {
		return ipc.Response{Ok: false, Error: fmt.Sprintf("reload refused, config kept as is: %v", err)}
	}
	// Give the IPC response a chance to reach the GUI before this process exits.
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
	return ipc.Response{Ok: true, PID: p.Pid}
}

func ipcGetConfig() (map[string]any, error) {
	tree, err := cfg.ReadTOMLTree(CONFIG_PATH)
	if err != nil {
		return nil, err
	}
	return tree.ToMap(), nil
}

func ipcSetConfig(mods map[string]any) error {
	normalized := make(map[string]interface{}, len(mods))
	for key, value := range mods {
		// JSON turns every TOML integer into float64; go-toml refuses plain
		// ints on Marshal, so restore them as int64.
		if f, ok := value.(float64); ok {
			if i := int64(f); float64(i) == f {
				value = i
			}
		}
		normalized[key] = value
	}
	return cfg.WriteTOMLChecked(CONFIG_PATH, normalized)
}

// configKillsChild reports whether a respawned child would die on the config that is on
// disk: the file is read and given to the same parser the daemon boots with, so invalid
// TOML and an [Action.*] cycle are both refused here, before the parent lets go. A file
// that cannot be read is NOT fatal -- config() logs it and comes up on the embedded
// defaults, which is how the parent itself booted; refusing that would also stop the
// startup DE-wait re-exec, and a user with no config file at all still needs it, because
// without the re-exec the DE unmaps the keys xswitcher grabbed.
//
// What this gate cannot see is everything the parser leaves for the daemon: sequences()
// and keys() panic AFTER it returned -- an invalid WordChars/WordHead/OFF/ON/SEQ regex, a
// duplicated OFF/ON/SEQ declaration, an unresolvable key name -- and serve() panics on a
// [ScanDevices] Test path that is not there. Those are not the parent's to predict, they
// are the child's to survive, and handover() is what finds out: each of them kills the
// child before it binds the GUI socket, so the parent sees the death, keeps the keyboard
// it already has, and reports the refusal instead of leaving the user without one.
func configKillsChild() error {
	data, err := os.ReadFile(CONFIG_PATH)
	if err != nil {
		return nil
	}
	if _, err = cfg.ParseTOML(data); err != nil {
		return fmt.Errorf("%s would kill the respawned child: %w", CONFIG_PATH, err)
	}
	return nil
}

// validateConfig is the strict version the GUI asks for: here an unreadable file is worth
// reporting, because the user is about to be told their settings are not on disk.
func validateConfig() error {
	data, err := os.ReadFile(CONFIG_PATH)
	if err != nil {
		return err
	}
	_, err = cfg.ParseTOML(data)
	return err
}

func clipboardText() (string, bool) { // The clipboard's text, or false if it is unreadable or holds none.
	// Init only has to succeed once per process, so the guard lives here instead of at every caller.
	// The content itself is never printed: it is whatever the user last copied.
	if ! clipboardOk {
		if err := clipboard.Init(); err != nil {
			fmt.Printf("Unable to read from the clipboard: %v.\n", err)
			return "", false
		}
		clipboardOk = true
	}
	b := clipboard.Read(clipboard.FmtText)
	if b == nil { return "", false }
	return string(b), true
}

func typeClipboard(A *TAction) { // Try to type the text from clipboard to virtual keyboard.
	b, ok := clipboardText()
	if !ok { return }
	typeText(b)
}

func serve() {
	var event t_key

	stat, err := os.Stat(ScanDevices.Test)
	if err != nil {
		panic(fmt.Sprintf("Device test error: Unable to stat device: %s", err))
	}
	now := time.Now()

	// Must wait for DE to be started. Otherwise, DE sees stupid keyboard and unmaps my rigth alt|win keys at all!
	if now.Sub(stat.ModTime()) < (time.Duration(ScanDevices.Respawn) * time.Second) {
		go func() {
			time.Sleep((time.Duration(ScanDevices.Respawn) * time.Second) - now.Sub(stat.ModTime()))
			Respawn(nil)
		}()
	}

	display = C.XOpenDisplay(nil);
	if display == nil {
		panic("Error while XOpenDisplay()!")
	}
	// Set callback https://stackoverflow.com/questions/32947511/cgo-cant-set-callback-in-c-struct-from-go
	C.set_handle_error()
	x_class = C.XAllocClassHint()

	// Everything that can stop this process is behind us: the config, the compiled
	// expressions, the keys, the device scan, the virtual keyboard, inotify and the X11
	// connection. Binding the GUI socket now, and not before, is what makes owning it a
	// boot proof for the parent that respawned us -- see listenIPC() in handover.go.
	listenIPC()

	for {
		select {
		case event = <- miceEvents: // code is always 0x4, while value is 0x90000 + button(1,2,3...)
			if WC != nil && WC.MouseClickDrops {
				dropKey(event)
			}
		case event = <- keyboardEvents:
			if event.code > 767 { // Upper bound of key_name[]; the field is uint16, so `code < 0` was always false.
				fmt.Printf("!!! Invalid event code: %d\n", event.code);
			} else {
				if *TEST_MODE {
					fmt.Fprintf(os.Stderr, ",%s:%d", key_name[event.code], event.value)
					continue
				}
				KEYS[event.code](event)
			}
		}
		updateStatusSnapshot()
	}
}

func main() {
	var err error
	defer func() { // Report panic, if one occured
		if *DEBUG { return } // StackTrace is only interesting along debug
		if r := recover(); r != nil {
			fmt.Printf("%v\n", r)
		}
	}()

	// Hooks hashtable
	ACTIONS["DropBuffers"] = DropBuffers // Drop all buffers
	ACTIONS["Clean"] = Clean             // New word (clean word buffers)
	ACTIONS["RetypeWord"] = RetypeWord
	ACTIONS["Switch"] = Switch
	ACTIONS["Layout"] = Layout
	ACTIONS["Respawn"] = Respawn
	ACTIONS["TypeClipboard"] = typeClipboard // Try to type the text from clipboard to virtual keyboard
	ACTIONS["Exec"] = Exec

	config() // Parse config
	sequences() // Compile expressions
	keys() // Initialize key actions

	// The IPC server for the GUI is built here but bound at the end of serve(), the last
	// step that can still fail on the config: owning the socket then means having booted,
	// and handover() is what waits for that. Building it early stays required because the
	// key-event goroutines below call sendSound through ipcSrv -- assigning it before
	// connectEvents removes the race without a mutex.
	sockPath := ipc.SocketPath()
	srv := ipc.NewServer(sockPath, ipcStatus, ipcReload)
	srv.SetConfigHandler(ipcGetConfig, ipcSetConfig, validateConfig)
	srv.SetDiaryHandler(ipcGetDiary)
	ipcSrv = srv

	connectEvents("") // Start keyloggers

	// Seed the status snapshot so the first GUI query sees the attached devices
	// even before the first key event runs the serve loop.
	updateStatusSnapshot()

	// Attach virtual keyboard
	kb, err = keybd_event.NewKeyBonding()
	if err != nil {
		panic(fmt.Sprintf("Virtual keyboard error: %s", err))
	}

	// Create new inotify watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(fmt.Sprintf("Inotify error in NewWatcher(): %s", err))
	}
	defer watcher.Close()

    go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					fmt.Printf("***WTF***: read(watcher.Events) != ok\n")
					continue
				}
				if event.Has(fsnotify.Create) {
					fmt.Printf("New input device: %s\n", event.Name)
					time.Sleep(1000 * time.Millisecond)
					connectEvents(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					fmt.Printf("***WTF***: read(watcher.Errors) != ok\n")
					continue
				}
				fmt.Printf("watcher error: %v\n", err)
			}
		}
	}()

	// inotify on "/dev/input"
	err = watcher.Add("/dev/input")
	if err != nil {
		panic(fmt.Sprintf("Inotify error in Add(\"/dev/input\"): %s", err))
	}

	serve() // Main loop
}
