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
	flag "github.com/spf13/pflag"       // CLI keys like python's "argparse". More flexible, comparing with "gnuflag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
//	"sort"
	"strings"
	"strconv"
	"syscall"
	"time"
//	"unsafe"
	"github.com/pelletier/go-toml"      // Actual TOML parser
	"github.com/gvalkov/golang-evdev"   // Keyboard and mouse events
	"github.com/micmonay/keybd_event"   // Virtual keyboard !!(must be improved to deal with complex input)
)

// Config
type TScanDevices struct {
	Test string     `default:"/dev/input/event0"`
	Respawn int     `default:"30"`
	Search string   `default:"/dev/input/event*"`
	Bypass string   `default:"(?i)Video|Camera"`
	BypassRE *(regexp.Regexp)
}

type TActionKeys struct {
	Layouts []int `default:"[0,1]"`
	Add []string
	Drop []string
	Test []string
	StateKeys []string
}

type TWindowClass struct {
	Regex string
	re *(regexp.Regexp)
	MouseClickDrops bool
	Actions string
}

type TActions struct {
	SeqLength int   `default:8`
	WordChars string `default:"(^([0-9A-Z=-]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9]):0$)"`
	WordHead string `default:"(^([0-9A-Z]|GRAVE|APOSTROPHE|SEMICOLON|[LR]_BRACE|COMMA|DOT|(BACK)?SLASH|KP[0-9]):1$)"`
	NewWord []string
	NewSentence []string
	Compose []string
	Custom map[string][]string // >> TAction.Name
}

type TAction struct {
	Action []string
	Layout int `default:"-1"`
	Layouts []int
	Exec string
	Wait bool
	SendBuffer string
}

type TSequence struct {
	OFF * regexp.Regexp
	ON * regexp.Regexp
	SEQ * regexp.Regexp
}

type TSequences []TSequence

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

	debug bool
	DEBUG *bool = &debug

	verbose bool
	VERBOSE *bool = &verbose

	test_mode bool
	TEST_MODE *bool = &test_mode

	// Config
	ScanDevices TScanDevices
	Templates map [string]string
	ActionKeys TActionKeys
	WindowClasses []TWindowClass
	Actions TActions
	ActionSet map[string] TAction

	// Parse by regex
	ON *(regexp.Regexp) = regexp.MustCompile("(\\s|^)ON:\\([^\\s]+\\)") // "ON:(CTRL|ALT|META)"
	OFF *(regexp.Regexp) = regexp.MustCompile("(\\s|^)OFF:\\([^\\s]+\\)") // "OFF:(CTRL|ALT|META)"
	SEQ *(regexp.Regexp) = regexp.MustCompile("(\\s|^)SEQ:\\([^\\s]+\\)") // "SEQ:(@WORD@:2,@WORD@:0)"
	TEMPLATE *(regexp.Regexp) = regexp.MustCompile("@[A-Za-z0-9_]+@") // "@WORD@"

	Action = regexp.MustCompile("^Action\\..+")
	ActionName = regexp.MustCompile("\\..+$")
	WordChars *(regexp.Regexp) // Actions.WordChars
	WordHead *(regexp.Regexp) // Actions.WordHead

	// Sequences
	NewWord TSequences
	NewSentence TSequences
	Compose TSequences
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
	DOWN = make(map[uint16] int) // In any case, all virtual keys MUST BE RELEASED at the end of retyping.
)

func config() {
	var (
		conf map [string]interface{}
		conf_ []byte
		err error
	)
	config_path := &CONFIG_PATH

	if env_config, ok := os.LookupEnv("CONFIG"); ok {
		*config_path = env_config
	}
	_ , *DEBUG = os.LookupEnv("DEBUG")
	_ , *VERBOSE = os.LookupEnv("VERBOSE")
	_ , *TEST_MODE = os.LookupEnv("TEST")

	F := flag.NewFlagSet("", flag.ContinueOnError)
	config_path = F.StringP("conf", "c", *config_path, "Non-default config location")
	DEBUG = F.BoolP("debug", "d", *DEBUG, "Debug log level")
	VERBOSE = F.BoolP("verbose", "v", *VERBOSE, "Increase log level to NOTICE")
	TEST_MODE = F.BoolP("test", "t", *TEST_MODE, "Only output all key events to STDERR. No actions.")
	F.Init("", flag.ExitOnError)
	F.Parse(os.Args[1:])

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
	if err := toml.Unmarshal(conf_, &conf); err != nil {
		panic(fmt.Errorf("Config error: unable to parse config file:\n%s", err.Error()))
	}

	for key, value := range conf {
		switch key {
		case "ScanDevices":
			_ScanDevices, err := toml.Marshal(value)
			if err != nil {
				panic(fmt.Errorf("Config error: unable to parse [ScanDevices]:\n%s", err.Error()))
			}
			if err = toml.Unmarshal(_ScanDevices, &ScanDevices); err != nil {
				panic(fmt.Errorf("Config error: unable to parse [ScanDevices]:\n%s", err.Error()))
			}
			if ScanDevices.BypassRE, err = regexp.Compile(ScanDevices.Bypass); err != nil {
				panic(fmt.Errorf("Config error: unable to parse [ScanDevices]. Invalid regexp for \"Bypass\".\n%s", err.Error()))
			}
		case "Templates":
			switch t := value.(type) {
			case map[string]interface{}:
				Templates = make(map [string]string, len(t))
				for k, v := range t {
					if _, err = regexp.Compile(v.(string)); err != nil {
						panic(fmt.Errorf("Config error: unable to parse [Templates]. Invalid regexp for \"%s\".\n%s", k, err.Error()))
					}
					Templates[k] = v.(string)
				}
			default:
				panic(fmt.Errorf("Config error: [Templates] must consist of \"name = string\" peers "))
			}

		case "ActionKeys":
			_ActionKeys, err := toml.Marshal(value)
			if err != nil {
				panic(fmt.Errorf("Config error: unable to parse [ActionKeys]:\n%s", err.Error()))
			}
			if err = toml.Unmarshal(_ActionKeys, &ActionKeys); err != nil {
				panic(fmt.Errorf("Config error: unable to parse [ActionKeys]:\n%s", err.Error()))
			}

		case "WindowClasses":
			switch t := value.(type) {
			case []map[string]interface{}:
				for _, v := range t {
					_WindowClass, err := toml.Marshal(v)
					if err != nil {
						panic(fmt.Errorf("Config error: unable to parse [[WindowClasses]]:\n%s", err.Error()))
					}
					class :=TWindowClass{}
					if err = toml.Unmarshal(_WindowClass, &class); err != nil {
						panic(fmt.Errorf("Config error: unable to parse [[WindowClasses]]:\n%s", err.Error()))
					}

					if class.Regex != "" {
						class.re, err = regexp.Compile(class.Regex)
						if err != nil {
							panic(fmt.Errorf("Config error: unable to parse [[WindowClasses]]. Invalid regexp \"%s\".\n%s", class.Regex, err.Error()))
						}
					} else {
						class.re = nil
					}

					if class.Actions != "" {
						if class.Actions != "Actions" { // Too complecs for the first release.
							panic(fmt.Errorf("Config error: Only one action set \"Actions\" is realised for [[WindowClasses]] in current version.\nUnable to  serve \"%s\"", class.Actions))
						}
					}
					WindowClasses = append(WindowClasses, class)
				}
			default:
				panic(fmt.Errorf("Config error: [[WindowClasses]] must be a slice of sections, not a single [WindowClasses] section"))
			}

		case "Actions":
			switch t := value.(type) {
			case map[string]interface{}:
				Actions.Custom = make(map[string][]string, len(t) - 4) // 4 = count of preset keys
				for k, v := range t {
					switch k {
					case "SeqLength":
						switch t := v.(type) {
						case int64:
							if (t < 1) || (t > 255) {
								panic(fmt.Errorf("Config error: Actions.SeqLength must be integer betwen 1 and 255"))
							}
							Actions.SeqLength = int(t)
						default:
							panic(fmt.Errorf("Config error: Actions.SeqLength must be integer betwen 1 and 255"))
						}
					case "WordChars":
						switch t := v.(type) {
						case interface{}:
							Actions.WordChars = t.(string)
						default:
							panic(fmt.Errorf("Config error: \"WordChars\" value must be string"))
						}
					case "WordHead":
						switch t := v.(type) {
						case interface{}:
							Actions.WordHead = t.(string)
						default:
							panic(fmt.Errorf("Config error: \"WordHead\" value must be string"))
						}
					case "NewWord":
						switch t := v.(type) {
						case []interface{}:
							for _, seq := range t {
								Actions.NewWord = append(Actions.NewWord, seq.(string))
							}
						default:
							panic(fmt.Errorf("Config error: \"NewWord\" value must be array of strings"))
					}
					case "NewSentence":
						switch t := v.(type) {
						case []interface{}:
							for _, seq := range t {
								Actions.NewSentence = append(Actions.NewSentence, seq.(string))
							}
						default:
							panic(fmt.Errorf("Config error: \"NewSentence\" value must be array of strings"))
					}
					case "Compose":
						switch t := v.(type) {
						case []interface{}:
							for _, seq := range t {
								Actions.Compose = append(Actions.Compose, seq.(string))
							}
						default:
							panic(fmt.Errorf("Config error: \"Compose\" value must be array of strings"))
					}
					default:
						if Action.MatchString(k) {
							switch t := v.(type) {
							case []interface{}:
								name := strings.TrimLeft(ActionName.FindString(k), ".")
								for _, seq := range t {
									Actions.Custom[name] = append(Actions.Custom[name], seq.(string))
								}
							default:
								panic(fmt.Errorf("Config error: \"Action.\" values must be arrays of strings"))
							}
						} else {
							panic(fmt.Errorf("Config error: unknown key \"%s\" in [Actions] section", k))
						}
					}
				}

			default:
				panic(fmt.Errorf("Config error: [Actions] must consist of \"name = value\" peers "))
			}

		case "Action":
			switch t := value.(type) {
			case map[string]interface{}:
				ActionSet = make(map[string] TAction, len(t))
				for key, value := range t {
					_Action, err := toml.Marshal(value)
					if err != nil {
						panic(fmt.Errorf("Config error: unable to parse [Action.%s]:\n%s", key, err.Error()))
					}
					act := TAction{}
					if err = toml.Unmarshal(_Action, &act); err != nil {
						panic(fmt.Errorf("Config error: unable to parse [Action.%s]:\n%s", key, err.Error()))
					}
					ActionSet[key] = act
				}
			default:
				panic(fmt.Errorf("Config error: [[WindowClasses]] must be a slice of sections, not a single [WindowClasses] section %T", t))
			}

		default:
			panic(fmt.Errorf("Config error: unknown section name [%s]", key))
		}
	}
	// Finally, check that ActionSet contains all Custom Actions
	for key, _ := range Actions.Custom {
		if _, ok := ActionSet[key]; !ok {
			panic(fmt.Errorf("Config error: Action definition not found for \"Action.%s\"", key))
		}
	}
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

	ActSeq = make(map[string] TSequences)
	for key, value := range Actions.Custom {
		for _, s := range value {
			ActSeq[key] = append(ActSeq[key], seqParse(s, key))
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
		key_name[code] = name
	}
	// Extra key aliases
	key_def["MINUS"] = 12
	key_def["EQUAL"] = 13
	key_def["["] = 26
	key_def["]"] = 27
	key_def[";"] = 39
	key_def["'"] = 40
	key_def["`"] = 41
	key_def["\\"] = 43
	key_def[","] = 51
	key_def["."] = 52
	key_def["/"] = 53

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
	if C.XGetInputFocus(display, &ActiveWindowId, &revert_to) == 0 {
		ActiveWindowId = 0
		fmt.Println("ActiveWindowId = 0", C.XGetInputFocus(display, &ActiveWindowId, &revert_to))
	}

	if ActiveWindowId == ActiveWindowId_old { return false}

	if ActiveWindowId <= 1 {
		ActiveWindowClass = ""
		fmt.Println("ActiveWindowId <= 1")
		return
	}

	// !!! "X Error of failed request:  BadWindow (invalid Window parameter)" in case of window was gone.
	// https://eli.thegreenplace.net/2019/passing-callbacks-and-pointers-to-cgo/
	// https://artem.krylysov.com/blog/2017/04/13/handling-cpp-exceptions-in-go/
	// >>> https://stackoverflow.com/questions/32947511/cgo-cant-set-callback-in-c-struct-from-go
//	C.XFlush(display)
	if C.XGetClassHint(display, ActiveWindowId, x_class) > 0 { // "VirtualBox Machine"
		if ActiveWindowClass != C.GoString(x_class.res_name) {
			ActiveWindowClass = C.GoString(x_class.res_name)
			fmt.Println("=", ActiveWindowClass)
		}
	} else {
		if C.XGetClassHint(display, ActiveWindowId - 1, x_class) > 0 { // https://antofthy.gitlab.io/info/X/WindowID.txt
			// However the reported ID is generally wrong for GTK apps (like Firefox) and the windows immediate parent is actually needed...
			// Typically for GTK the parent window is 1 less than the focus window ... But there is no gurantee that the ID is one less.
			if ActiveWindowClass != C.GoString(x_class.res_name) {
				ActiveWindowClass = C.GoString(x_class.res_name)
				fmt.Println("*", ActiveWindowClass)
			}
		} else {
			fmt.Println("Empty ActiveWindowClass. M.b. gnome \"window\"?")
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
}

func checkLanguageId() bool {
	state := new(C.struct__XkbStateRec)
	C.XkbGetState(display, C.XkbUseCoreKbd, state);

	for _, l := range ActionKeys.Layouts {
		if l == int(state.group) { return true }
	}

	return false
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
	state := new(C.struct__XkbStateRec)
	layout := C.uint(0)

	C.XkbGetState(display, C.XkbUseCoreKbd, state);
	if lang >= 0 {
		layout = C.uint(lang)
		C.XkbLockGroup(display, C.XkbUseCoreKbd, layout);
		C.XkbGetState(display, C.XkbUseCoreKbd, state);
	}

	return int(state.group)
}

// Push or release the key on virtual keyboard
func sendKey(key t_key) {
	switch key.value {
	case 0:
		kb.Up(key.code)
		kb.Sync()
	default:
		kb.Down(key.code)
		kb.Sync()
	}
	time.Sleep(5 * time.Millisecond)
}

// Single key press on virtual keyboard
func pressKey(key int) {
	sendKey(t_key{uint16(key), 1})
	sendKey(t_key{uint16(key), 0})
}

// ACTIONS (RetypeWord, Switch, Layout, Respawn, Exec)
func RetypeWord(A *TAction) {
	if (len(WORD) - EXTRA) < 1 { // WTF?!
		fmt.Println("RetypeWord error: WORD is smaller than EXTRA!", EXTRA)
		newWord()
		return
	}
	count := 0 // Count chars to be deleted
	seq := 0 // Incremental key event counter
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
	for i := 0; i < count - COMPOSE; i++ {
		pressKey(evdev.KEY_BACKSPACE)
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
		fmt.Printf("RETYPE: %v", CTRL_WORD) // Helpfull to debug e.g. keyboard bounce
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

	// Clear virtual keyboard state
	if len(DOWN) > 0 {
		fmt.Println("RetypeWord warning: found pushed keys after retyping was done! %v", DOWN)
		for k, _ := range DOWN {
			sendKey(t_key{k, 0})
		}
	}
}

// ToDo: newWord() | dropBuffers() must be implemented here in "smart" way.
// Nested action: leave buffers as is. Or implement extra option "what to do with buffers".
// Single action: drop. Or be smarter and remember the layout inside key sequences...
func Switch(A *TAction) {
	next := 0
	l := Language(-1)

	for i := 0; i < len(A.Layouts); i++ {
		if l == A.Layouts[i] {
			next = l + 1
		}
	}

	if next >= len(A.Layouts) {
		next = 0
	}

	Language(A.Layouts[next])
}

func Layout(A *TAction) {
	Language(A.Layout)
}

func Exec(A *TAction) {

}

// Perform actions depending on WindowClasses[]
func setWindowActions() {
	WC = nil
	// Depending on WindowClass
	for _, w := range WindowClasses {
		if w.re != nil {
			if ActiveWindowClass == "" { // Empty class name match empty regex
				continue
			} else {
				if w.re.MatchString(ActiveWindowClass) {
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
	if testAction(&NewWord) {
		if *VERBOSE || *DEBUG {
			fmt.Printf("NewWord: %s\n", TAIL.String()[1:])
		}
		newWord()
		return
	}
	for name, act := range ActSeq { // Is there some reason to do more then 1 action?
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

	if ! checkLanguageId() { return } // Don't proceed with extra languages


	if getActiveWindowId() { // New focused window detected
		// Drop buffers, but store this event
		dropBuffers()
		setWindowActions()
	}

	for _, key := range slice {
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


func mouse(device *evdev.InputDevice) {
	for {
		event, err := device.ReadOne()
		if err != nil {
			fmt.Printf("Closing device \"%s\" due to an error:\n\"\"\" %s \"\"\"\n", device.Name, err.Error())
			return
		}

		if event.Type == evdev.EV_MSC { // Button events
			miceEvents <- t_key{event.Code, event.Value}
		}
	}
}


func keyboard(device *evdev.InputDevice) {
	for {
		event, err := device.ReadOne()
		if err != nil {
			fmt.Printf("Closing device \"%s\" due to an error:\n\"\"\" %s \"\"\"\n", device.Name, err.Error())
			return
		}

		if event.Type == evdev.EV_KEY { // Key events
			keyboardEvents <- t_key{event.Code, event.Value}
		}
	}
}

func connectEvents() {
	var (
		is_mouse bool
		is_keyboard bool
		skip_it bool
	)

	dev, err := evdev.ListInputDevices(ScanDevices.Search)
	if err != nil {
		panic(fmt.Sprintf("Events error: Unable to list devices: %s", err.Error()))
	}

	for _, device := range dev {
		is_mouse = false
		is_keyboard = false
		for ev := range device.Capabilities {
			switch ev.Type {
			case evdev.EV_ABS, evdev.EV_REL:
				is_mouse = true
				continue
			case evdev.EV_KEY:
				is_keyboard = true
				continue
			case evdev.EV_SYN, evdev.EV_MSC, evdev.EV_SW, evdev.EV_LED, evdev.EV_SND: // EV_SND == "Eee PC WMI hotkeys"
			default:
				skip_it = true
				fmt.Printf("Events warning: Skipping device \"%s\" because it has unsupported event type: %x", device.Name, ev.Type)
			}
		}

		if skip_it || ScanDevices.BypassRE.MatchString(device.Name) { continue }

		if is_mouse {
			fmt.Println("mouse:", device.Name)
			go mouse(device)
		} else if is_keyboard {
			fmt.Println("keyboard:", device.Name)
			go keyboard(device)
		}
	}
	return
}

// https://gravitational.com/blog/golang-ssh-bastion-graceful-restarts/
func forkChild() (*os.Process, error) {
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
	p, err := os.StartProcess(execName, []string{execName}, &os.ProcAttr{
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
		fmt.Printf("Unable to fork child: %v.\n", err)
		return
	}
	fmt.Printf("Forked child %v.\n", p.Pid)
	os.Exit(0)
}

func serve() {
	var event t_key

	stat, err := os.Stat(ScanDevices.Test)
	if err != nil {
		panic(err)
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

	for {
		select {
		case event = <- miceEvents: // code is always 0x4, while value is 0x90000 + button(1,2,3...)
			if WC != nil && WC.MouseClickDrops {
				dropKey(event)
			}
		case event = <- keyboardEvents:
			if event.code < 0 || event.code > 767 { // Fuse against out-of-bounds: in old times there was need in.
				fmt.Printf("!!! Invalid event code: %d\n", event.code);
			} else {
				if *TEST_MODE {
					fmt.Fprintf(os.Stderr, ",%s:%d", key_name[event.code], event.value)
					continue
				}
				KEYS[event.code](event)
			}
		}
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
	ACTIONS["RetypeWord"] = RetypeWord
	ACTIONS["Switch"] = Switch
	ACTIONS["Layout"] = Layout
	ACTIONS["Respawn"] = Respawn
	ACTIONS["Exec"] = Exec

	config() // Parse config
	sequences() // Compile expressions
	keys() // Initialize key actions

	connectEvents() // Start keyloggers

	// Attach virtual keyboard
	kb, err = keybd_event.NewKeyBonding()
	if err != nil {
		panic(err)
	}

	serve() // Main loop
}
