// Package config — see types.go for overview.
// parse.go extracts the TOML→struct conversion from src/xswitcher.go:config()
// so both the daemon and the GUI can share it without pulling cgo into the GUI.
package config

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml"
)

// ParsedConfig is the result of ParseTOML. The daemon copies these fields
// into its own globals; the GUI uses them directly for editing.
type ParsedConfig struct {
	ScanDevices   TScanDevices
	Keyboard      TKeyboard
	Templates     map[string]string
	ActionKeys    TActionKeys
	WindowClasses []TWindowClass
	Actions       TActions
	ActionSet     map[string]TAction
	Wayland       TWayland
	AutoReplace   []TAutoReplace
	Hotkeys       []THotkey
	Sounds        TSounds
	Diary         TDiary
	AutoDetect    TAutoDetect
}

// SoundEvents are the event kinds the daemon emits; [Sounds] may only bind
// files for these, so a typo in an event name fails validation instead of
// silently producing no sound.
var SoundEvents = map[string]bool{
	"layout_switch": true,
	"retype":        true,
	"auto_replace":  true,
	"error":         true,
}

// AutoDetectLanguages are the language codes [AutoDetect] Languages may name.
// They are the models internal/autodetect ships with and the corpus files under
// internal/autodetect/corpus/ are named after them, so a code with no table
// would make the section silently decide nothing.
var AutoDetectLanguages = map[string]bool{
	"en": true,
	"ru": true,
}

// DefaultAutoDetect returns the [AutoDetect] values a section that names no keys
// runs with. Those numbers are written in exactly one place -- the struct tags --
// and go-toml reads them during Unmarshal, so this asks the same unmarshal
// instead of repeating 5 and 0.2: the GUI's preview has to judge a word by the
// pair the daemon uses, and a second copy of the numbers could go stale with
// nothing failing.
//
// The tags only apply through this path. A file with no [AutoDetect] section at
// all parses to the zero struct, because ParseTOML unmarshals a section it found
// and never the document as a whole -- measured, not assumed. Enabled staying
// false is what makes that harmless: without it no word is judged at all.
func DefaultAutoDetect() TAutoDetect {
	var ad TAutoDetect
	// An empty document has nothing to fail on; a failure here means the tags
	// stopped being read, which is a bug in the binary and must not hand the
	// caller zeros that silently drop both brakes.
	if err := toml.Unmarshal([]byte(""), &ad); err != nil {
		panic(fmt.Sprintf("config: [AutoDetect] defaults do not parse: %v", err))
	}
	return ad
}

// HotkeyModifiers are the key names a [[Hotkeys]] combo may list as held
// conditions; everything else in Keys is the single trigger key. The names
// are the daemon's key_def spellings, because that is what CTRL stores.
var HotkeyModifiers = map[string]bool{
	"L_CTRL": true, "R_CTRL": true,
	"L_SHIFT": true, "R_SHIFT": true,
	"L_ALT": true, "R_ALT": true,
	"L_META": true, "R_META": true,
	"CAPS": true,
}

// ActionMatchRe matches "Action.<name>" keys inside [Actions].
var ActionMatchRe = regexp.MustCompile(`^Action\..+`)

// ActionNameRe extracts the name after the dot in "Action.<name>".
var ActionNameRe = regexp.MustCompile(`\..+$`)

// ActionsCustomHint sizes the initial capacity of Actions.Custom. Exported
// so the daemon's test file can verify the boundary conditions after the
// function was extracted from src/xswitcher.go.
func ActionsCustomHint(n int) int {
	if n < 4 {
		return 0
	}
	return n - 4
}

// ParseTOML converts raw TOML bytes into a ParsedConfig. It does NOT touch
// any daemon globals or compiled runtime state beyond what the structs carry
// (BypassRE, WindowClass.RE). Those are the caller's responsibility.
func ParseTOML(data []byte) (*ParsedConfig, error) {
	var conf map[string]interface{}
	if err := toml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("unable to parse config file:\n%s", err.Error())
	}

	out := &ParsedConfig{}

	for key, value := range conf {
		switch key {
		case "ScanDevices":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [ScanDevices]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.ScanDevices); err != nil {
				return nil, fmt.Errorf("unable to parse [ScanDevices]:\n%s", err.Error())
			}
			if out.ScanDevices.BypassRE, err = regexp.Compile(out.ScanDevices.Bypass); err != nil {
				return nil, fmt.Errorf("[ScanDevices] invalid regexp for \"Bypass\":\n%s", err.Error())
			}

		case "Templates":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[Templates] must consist of \"name = string\" pairs")
			}
			out.Templates = make(map[string]string, len(t))
			for k, v := range t {
				s, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("[Templates] value for \"%s\" must be a string", k)
				}
				if _, err := regexp.Compile(s); err != nil {
					return nil, fmt.Errorf("[Templates] invalid regexp for \"%s\":\n%s", k, err.Error())
				}
				out.Templates[k] = s
			}

		case "ActionKeys":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [ActionKeys]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.ActionKeys); err != nil {
				return nil, fmt.Errorf("unable to parse [ActionKeys]:\n%s", err.Error())
			}

		case "WindowClasses":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[[WindowClasses]] must be a slice of sections, not a single [WindowClasses] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("unable to parse [[WindowClasses]]:\n%s", err.Error())
				}
				wc := TWindowClass{}
				if err = toml.Unmarshal(raw, &wc); err != nil {
					return nil, fmt.Errorf("unable to parse [[WindowClasses]]:\n%s", err.Error())
				}
				if wc.Regex != "" {
					wc.RE, err = regexp.Compile(wc.Regex)
					if err != nil {
						return nil, fmt.Errorf("[[WindowClasses]] invalid regexp \"%s\":\n%s", wc.Regex, err.Error())
					}
				}
				if wc.Actions != "" && wc.Actions != "Actions" {
					return nil, fmt.Errorf("only one action set \"Actions\" is realised for [[WindowClasses]]; unable to serve \"%s\"", wc.Actions)
				}
				out.WindowClasses = append(out.WindowClasses, wc)
			}

		case "AutoReplace":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[[AutoReplace]] must be a slice of sections, not a single [AutoReplace] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("unable to parse [[AutoReplace]]:\n%s", err.Error())
				}
				ar := TAutoReplace{}
				if err = toml.Unmarshal(raw, &ar); err != nil {
					return nil, fmt.Errorf("unable to parse [[AutoReplace]]:\n%s", err.Error())
				}
				if ar.Trigger == "" || ar.Replace == "" {
					return nil, fmt.Errorf("[[AutoReplace]] requires non-empty \"Trigger\" and \"Replace\"")
				}
				out.AutoReplace = append(out.AutoReplace, ar)
			}

		case "Hotkeys":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[[Hotkeys]] must be a slice of sections, not a single [Hotkeys] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("unable to parse [[Hotkeys]]:\n%s", err.Error())
				}
				hk := THotkey{}
				if err = toml.Unmarshal(raw, &hk); err != nil {
					return nil, fmt.Errorf("unable to parse [[Hotkeys]]:\n%s", err.Error())
				}
				triggers := 0
				for _, k := range hk.Keys {
					if k == "" {
						return nil, fmt.Errorf("[[Hotkeys]] \"Keys\" must not contain empty names")
					}
					if !HotkeyModifiers[k] {
						triggers++
					}
				}
				if triggers != 1 {
					return nil, fmt.Errorf("[[Hotkeys]] combo must contain exactly one non-modifier trigger key, got %d: %v", triggers, hk.Keys)
				}
				if hk.Action == "" {
					return nil, fmt.Errorf("[[Hotkeys]] \"Action\" must name an [Action.*] section")
				}
				out.Hotkeys = append(out.Hotkeys, hk)
			}

		case "Sounds":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[Sounds] must consist of \"event = file\" pairs")
			}
			out.Sounds = make(TSounds, len(t))
			for k, v := range t {
				s, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("[Sounds] value for \"%s\" must be a file path string", k)
				}
				if !SoundEvents[k] {
					return nil, fmt.Errorf("[Sounds] unknown event \"%s\" (known: layout_switch, retype, auto_replace, error)", k)
				}
				if s == "" {
					return nil, fmt.Errorf("[Sounds] \"%s\": the file path must not be empty", k)
				}
				out.Sounds[k] = s
			}

		case "Diary":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [Diary]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Diary); err != nil {
				return nil, fmt.Errorf("unable to parse [Diary]:\n%s", err.Error())
			}
			if out.Diary.Enabled && out.Diary.Path == "" {
				return nil, fmt.Errorf("[Diary] \"Path\" is required when Enabled is set")
			}
			if out.Diary.Path != "" && !strings.HasPrefix(out.Diary.Path, "/") {
				return nil, fmt.Errorf("[Diary] \"Path\" must be absolute: %q", out.Diary.Path)
			}
			if out.Diary.MaxSizeMB < 1 {
				return nil, fmt.Errorf("[Diary] \"MaxSizeMB\" must be at least 1")
			}
			if out.Diary.RotateCount < 0 {
				return nil, fmt.Errorf("[Diary] \"RotateCount\" must not be negative")
			}

		case "AutoDetect":
			// Confidence is the only float in the config, and go-toml v1 refuses
			// an integer where the struct wants a float64 -- measured: the key
			// written as "1" failed with `Can't convert 1(int64) to float64`,
			// while "1.0" is the same TOML number. The set_config path does the
			// same thing on its own (it turns an integral float back into an
			// int64), so the value is normalised before the struct is filled.
			if table, ok := value.(map[string]interface{}); ok {
				if n, ok := table["Confidence"].(int64); ok {
					table["Confidence"] = float64(n)
				}
			}
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [AutoDetect]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.AutoDetect); err != nil {
				return nil, fmt.Errorf("unable to parse [AutoDetect]:\n%s", err.Error())
			}
			// go-toml fills an absent key from its `default` tag (measured: a
			// [Diary] without MaxSizeMB arrives as 10), so the two thresholds
			// below only see the shipped defaults or what the user wrote.
			if out.AutoDetect.MinChars < 2 {
				return nil, fmt.Errorf("[AutoDetect] \"MinChars\" must be at least 2: a shorter run holds no letter bigram to score")
			}
			if out.AutoDetect.Confidence <= 0 || out.AutoDetect.Confidence > 1 {
				return nil, fmt.Errorf("[AutoDetect] \"Confidence\" must be above 0 and at most 1, got %v", out.AutoDetect.Confidence)
			}
			groups := make(map[int]bool, len(out.AutoDetect.Languages))
			langs := make(map[string]bool, len(out.AutoDetect.Languages))
			for _, item := range out.AutoDetect.Languages {
				c := strings.Split(item, ":")
				if len(c) != 2 {
					return nil, fmt.Errorf("[AutoDetect] Languages entry %q must be \"<group>:<language>\"", item)
				}
				g, err := strconv.Atoi(strings.TrimSpace(c[0]))
				if err != nil || g < 0 {
					return nil, fmt.Errorf("[AutoDetect] Languages entry %q: the group must be a non-negative integer", item)
				}
				lang := strings.TrimSpace(c[1])
				if !AutoDetectLanguages[lang] {
					return nil, fmt.Errorf("[AutoDetect] Languages entry %q: no model for %q (known: en, ru)", item, lang)
				}
				// One group per language and one language per group: the daemon
				// has to name the group a detected language lives in, and two
				// entries naming the same one would leave that undecided.
				if groups[g] {
					return nil, fmt.Errorf("[AutoDetect] Languages: group %d is named twice", g)
				}
				if langs[lang] {
					return nil, fmt.Errorf("[AutoDetect] Languages: language %q is named twice; the switch target would be ambiguous", lang)
				}
				groups[g], langs[lang] = true, true
			}
			if out.AutoDetect.Enabled && len(out.AutoDetect.Languages) < 2 {
				return nil, fmt.Errorf("[AutoDetect] \"Enabled\" needs at least two Languages entries; with one there is no layout to correct to")
			}

		case "Actions":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[Actions] must consist of \"name = value\" pairs")
			}
			out.Actions.Custom = make(map[string][]string, ActionsCustomHint(len(t)))
			for k, v := range t {
				switch k {
				case "SeqLength":
					n, ok := v.(int64)
					if !ok || n < 1 || n > 255 {
						return nil, fmt.Errorf("Actions.SeqLength must be integer between 1 and 255")
					}
					out.Actions.SeqLength = int(n)
				case "WordChars":
					s, ok := v.(string)
					if !ok {
						return nil, fmt.Errorf("\"WordChars\" value must be string")
					}
					out.Actions.WordChars = s
				case "WordHead":
					s, ok := v.(string)
					if !ok {
						return nil, fmt.Errorf("\"WordHead\" value must be string")
					}
					out.Actions.WordHead = s
				case "NewWord":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, fmt.Errorf("\"NewWord\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.NewWord = append(out.Actions.NewWord, seq.(string))
					}
				case "NewSentence":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, fmt.Errorf("\"NewSentence\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.NewSentence = append(out.Actions.NewSentence, seq.(string))
					}
				case "Compose":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, fmt.Errorf("\"Compose\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.Compose = append(out.Actions.Compose, seq.(string))
					}
				case "TypeClipboard":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, fmt.Errorf("\"TypeClipboard\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.TypeClipboard = append(out.Actions.TypeClipboard, seq.(string))
					}
				default:
					if ActionMatchRe.MatchString(k) {
						arr, ok := v.([]interface{})
						if !ok {
							return nil, fmt.Errorf("\"Action.\" values must be arrays of strings")
						}
						name := strings.TrimLeft(ActionNameRe.FindString(k), ".")
						for _, seq := range arr {
							out.Actions.Custom[name] = append(out.Actions.Custom[name], seq.(string))
						}
					} else {
						return nil, fmt.Errorf("unknown key \"%s\" in [Actions] section", k)
					}
				}
			}

		case "Action":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("[Action.*] must be a map of sections, got %T", value)
			}
			out.ActionSet = make(map[string]TAction, len(t))
			for key, val := range t {
				raw, err := toml.Marshal(val)
				if err != nil {
					return nil, fmt.Errorf("unable to parse [Action.%s]:\n%s", key, err.Error())
				}
				act := TAction{}
				if err = toml.Unmarshal(raw, &act); err != nil {
					return nil, fmt.Errorf("unable to parse [Action.%s]:\n%s", key, err.Error())
				}
				out.ActionSet[key] = act
			}

		case "Keyboard":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [Keyboard]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Keyboard); err != nil {
				return nil, fmt.Errorf("unable to parse [Keyboard]:\n%s", err.Error())
			}

		case "Wayland":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("unable to parse [Wayland]:\n%s", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Wayland); err != nil {
				return nil, fmt.Errorf("unable to parse [Wayland]:\n%s", err.Error())
			}

		default:
			return nil, fmt.Errorf("unknown section name [%s]", key)
		}
	}

	// Verify every custom action has a definition.
	for key := range out.Actions.Custom {
		if _, ok := out.ActionSet[key]; !ok {
			return nil, fmt.Errorf("action definition not found for \"Action.%s\"", key)
		}
	}

	// A hotkey may only bind to a section that exists.
	for _, hk := range out.Hotkeys {
		if _, ok := out.ActionSet[hk.Action]; !ok {
			return nil, fmt.Errorf("[[Hotkeys]] action definition not found for \"%s\"", hk.Action)
		}
	}

	// Auto-detection decides on the XKB group a word was typed in. Under
	// [Wayland] BypassX the daemon cannot read a group at all — Language() there
	// reports only the switch it made itself — so the section would act on a
	// guess. This cannot live in the case above: the two sections arrive in map
	// iteration order, which is random.
	if out.AutoDetect.Enabled && out.Wayland.BypassX {
		return nil, fmt.Errorf("[AutoDetect] cannot be Enabled together with [Wayland] BypassX = true: the layout is not observable there")
	}

	// A self-referential [Action.*] graph is as fatal as a syntax error: the daemon
	// expands "Action.xxx" by unbounded recursion, so the cycle overflows the stack on
	// the first keystroke matching the rule -- long after boot, far from the config
	// line that caused it. It lives here rather than in the daemon because this parser
	// is what the respawn gate (configKillsChild), the GUI's validate command and the
	// config writer all share, and a check only the daemon runs is a check the file
	// reaches the child having already passed.
	if err := checkActionCycles(out.ActionSet); err != nil {
		return nil, err
	}

	return out, nil
}

// checkActionCycles reports the first [Action.*] section that can be reached from
// itself. A value in some action's Action list reading "Action.<name>" is a reference
// doAction() follows, so the walk mirrors that expansion. Reaching a node by two paths
// is a diamond, not a cycle, hence the separate checked state. Roots are walked in
// sorted order because Go randomizes map iteration and the reported chain is the path
// from whichever root reached the cycle first -- the same file must name the same chain.
func checkActionCycles(set map[string]TAction) error {
	const (
		visiting = 1
		checked  = 2
	)
	state := make(map[string]int, len(set))
	var walk func(name, chain string) error // chain: the path from the root, ending with name
	walk = func(name, chain string) error {
		switch state[name] {
		case checked:
			return nil
		case visiting:
			return fmt.Errorf("\"Action.%s\" refers to itself: %s", name, chain)
		}
		state[name] = visiting
		for _, act := range set[name].Action {
			if ActionMatchRe.MatchString(act) {
				ref := strings.TrimLeft(ActionNameRe.FindString(act), ".")
				if err := walk(ref, chain+" -> "+ref); err != nil {
					return err
				}
			}
		}
		state[name] = checked
		return nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := walk(name, name); err != nil {
			return err
		}
	}
	return nil
}
