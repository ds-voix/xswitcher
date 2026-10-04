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

	"xswitcher/internal/l10n"
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
	GUI           TGUI
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
		return nil, l10n.Errf("config.parse.toml_syntax", "unable to parse config file:\n{{.Err}}", "Err", err.Error())
	}

	out := &ParsedConfig{}

	for key, value := range conf {
		switch key {
		case "ScanDevices":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.scandevices", "unable to parse [ScanDevices]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.ScanDevices); err != nil {
				return nil, l10n.Errf("config.parse.scandevices", "unable to parse [ScanDevices]:\n{{.Err}}", "Err", err.Error())
			}
			if out.ScanDevices.BypassRE, err = regexp.Compile(out.ScanDevices.Bypass); err != nil {
				return nil, l10n.Errf("config.parse.scandevices_bypass_regexp", "[ScanDevices] invalid regexp for \"Bypass\":\n{{.Err}}", "Err", err.Error())
			}

		case "Templates":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.templates_shape", "[Templates] must consist of \"name = string\" pairs")
			}
			out.Templates = make(map[string]string, len(t))
			for k, v := range t {
				s, ok := v.(string)
				if !ok {
					return nil, l10n.Errf("config.parse.templates_value_string", "[Templates] value for \"{{.Key}}\" must be a string", "Key", k)
				}
				if _, err := regexp.Compile(s); err != nil {
					return nil, l10n.Errf("config.parse.templates_regexp", "[Templates] invalid regexp for \"{{.Key}}\":\n{{.Err}}", "Key", k, "Err", err.Error())
				}
				out.Templates[k] = s
			}

		case "ActionKeys":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.actionkeys", "unable to parse [ActionKeys]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.ActionKeys); err != nil {
				return nil, l10n.Errf("config.parse.actionkeys", "unable to parse [ActionKeys]:\n{{.Err}}", "Err", err.Error())
			}

		case "WindowClasses":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.windowclasses_shape", "[[WindowClasses]] must be a slice of sections, not a single [WindowClasses] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, l10n.Errf("config.parse.windowclasses", "unable to parse [[WindowClasses]]:\n{{.Err}}", "Err", err.Error())
				}
				wc := TWindowClass{}
				if err = toml.Unmarshal(raw, &wc); err != nil {
					return nil, l10n.Errf("config.parse.windowclasses", "unable to parse [[WindowClasses]]:\n{{.Err}}", "Err", err.Error())
				}
				if wc.Regex != "" {
					wc.RE, err = regexp.Compile(wc.Regex)
					if err != nil {
						return nil, l10n.Errf("config.parse.windowclasses_regexp", "[[WindowClasses]] invalid regexp \"{{.Regex}}\":\n{{.Err}}", "Regex", wc.Regex, "Err", err.Error())
					}
				}
				if wc.Actions != "" && wc.Actions != "Actions" {
					return nil, l10n.Errf("config.parse.windowclasses_actions", "only one action set \"Actions\" is realised for [[WindowClasses]]; unable to serve \"{{.Actions}}\"", "Actions", wc.Actions)
				}
				out.WindowClasses = append(out.WindowClasses, wc)
			}

		case "AutoReplace":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.autoreplace_shape", "[[AutoReplace]] must be a slice of sections, not a single [AutoReplace] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, l10n.Errf("config.parse.autoreplace", "unable to parse [[AutoReplace]]:\n{{.Err}}", "Err", err.Error())
				}
				ar := TAutoReplace{}
				if err = toml.Unmarshal(raw, &ar); err != nil {
					return nil, l10n.Errf("config.parse.autoreplace", "unable to parse [[AutoReplace]]:\n{{.Err}}", "Err", err.Error())
				}
				if ar.Trigger == "" || ar.Replace == "" {
					return nil, l10n.Errf("config.parse.autoreplace_fields", "[[AutoReplace]] requires non-empty \"Trigger\" and \"Replace\"")
				}
				out.AutoReplace = append(out.AutoReplace, ar)
			}

		case "Hotkeys":
			arr, ok := value.([]map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.hotkeys_shape", "[[Hotkeys]] must be a slice of sections, not a single [Hotkeys] section")
			}
			for _, v := range arr {
				raw, err := toml.Marshal(v)
				if err != nil {
					return nil, l10n.Errf("config.parse.hotkeys", "unable to parse [[Hotkeys]]:\n{{.Err}}", "Err", err.Error())
				}
				hk := THotkey{}
				if err = toml.Unmarshal(raw, &hk); err != nil {
					return nil, l10n.Errf("config.parse.hotkeys", "unable to parse [[Hotkeys]]:\n{{.Err}}", "Err", err.Error())
				}
				triggers := 0
				for _, k := range hk.Keys {
					if k == "" {
						return nil, l10n.Errf("config.parse.hotkeys_empty_keys", "[[Hotkeys]] \"Keys\" must not contain empty names")
					}
					if !HotkeyModifiers[k] {
						triggers++
					}
				}
				if triggers != 1 {
					return nil, l10n.Errf("config.parse.hotkeys_trigger_count", "[[Hotkeys]] combo must contain exactly one non-modifier trigger key, got {{.Count}}: {{.Keys}}", "Count", triggers, "Keys", hk.Keys)
				}
				if hk.Action == "" {
					return nil, l10n.Errf("config.parse.hotkeys_action_empty", "[[Hotkeys]] \"Action\" must name an [Action.*] section")
				}
				out.Hotkeys = append(out.Hotkeys, hk)
			}

		case "Sounds":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.sounds_shape", "[Sounds] must consist of \"event = file\" pairs")
			}
			out.Sounds = make(TSounds, len(t))
			for k, v := range t {
				s, ok := v.(string)
				if !ok {
					return nil, l10n.Errf("config.parse.sounds_value_string", "[Sounds] value for \"{{.Key}}\" must be a file path string", "Key", k)
				}
				if !SoundEvents[k] {
					return nil, l10n.Errf("config.parse.sounds_unknown_event", "[Sounds] unknown event \"{{.Event}}\" (known: layout_switch, retype, auto_replace, error)", "Event", k)
				}
				if s == "" {
					return nil, l10n.Errf("config.parse.sounds_empty_path", "[Sounds] \"{{.Key}}\": the file path must not be empty", "Key", k)
				}
				out.Sounds[k] = s
			}

		case "Diary":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.diary", "unable to parse [Diary]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Diary); err != nil {
				return nil, l10n.Errf("config.parse.diary", "unable to parse [Diary]:\n{{.Err}}", "Err", err.Error())
			}
			if out.Diary.Enabled && out.Diary.Path == "" {
				return nil, l10n.Errf("config.parse.diary_path_required", "[Diary] \"Path\" is required when Enabled is set")
			}
			if out.Diary.Path != "" && !strings.HasPrefix(out.Diary.Path, "/") {
				return nil, l10n.Errf("config.parse.diary_path_absolute", "[Diary] \"Path\" must be absolute: \"{{.Path}}\"", "Path", out.Diary.Path)
			}
			if out.Diary.MaxSizeMB < 1 {
				return nil, l10n.Errf("config.parse.diary_max_size", "[Diary] \"MaxSizeMB\" must be at least 1")
			}
			if out.Diary.RotateCount < 0 {
				return nil, l10n.Errf("config.parse.diary_rotate_count", "[Diary] \"RotateCount\" must not be negative")
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
				return nil, l10n.Errf("config.parse.autodetect", "unable to parse [AutoDetect]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.AutoDetect); err != nil {
				return nil, l10n.Errf("config.parse.autodetect", "unable to parse [AutoDetect]:\n{{.Err}}", "Err", err.Error())
			}
			// go-toml fills an absent key from its `default` tag (measured: a
			// [Diary] without MaxSizeMB arrives as 10), so the two thresholds
			// below only see the shipped defaults or what the user wrote.
			if out.AutoDetect.MinChars < 2 {
				return nil, l10n.Errf("config.parse.autodetect_min_chars", "[AutoDetect] \"MinChars\" must be at least 2: a shorter run holds no letter bigram to score")
			}
			if out.AutoDetect.Confidence <= 0 || out.AutoDetect.Confidence > 1 {
				return nil, l10n.Errf("config.parse.autodetect_confidence", "[AutoDetect] \"Confidence\" must be above 0 and at most 1, got {{.Value}}", "Value", out.AutoDetect.Confidence)
			}
			groups := make(map[int]bool, len(out.AutoDetect.Languages))
			langs := make(map[string]bool, len(out.AutoDetect.Languages))
			for _, item := range out.AutoDetect.Languages {
				c := strings.Split(item, ":")
				if len(c) != 2 {
					return nil, l10n.Errf("config.parse.autodetect_lang_shape", "[AutoDetect] Languages entry \"{{.Entry}}\" must be \"<group>:<language>\"", "Entry", item)
				}
				g, err := strconv.Atoi(strings.TrimSpace(c[0]))
				if err != nil || g < 0 {
					return nil, l10n.Errf("config.parse.autodetect_lang_group", "[AutoDetect] Languages entry \"{{.Entry}}\": the group must be a non-negative integer", "Entry", item)
				}
				lang := strings.TrimSpace(c[1])
				if !AutoDetectLanguages[lang] {
					return nil, l10n.Errf("config.parse.autodetect_lang_model", "[AutoDetect] Languages entry \"{{.Entry}}\": no model for \"{{.Language}}\" (known: en, ru)", "Entry", item, "Language", lang)
				}
				// One group per language and one language per group: the daemon
				// has to name the group a detected language lives in, and two
				// entries naming the same one would leave that undecided.
				if groups[g] {
					return nil, l10n.Errf("config.parse.autodetect_group_twice", "[AutoDetect] Languages: group {{.Group}} is named twice", "Group", g)
				}
				if langs[lang] {
					return nil, l10n.Errf("config.parse.autodetect_language_twice", "[AutoDetect] Languages: language \"{{.Language}}\" is named twice; the switch target would be ambiguous", "Language", lang)
				}
				groups[g], langs[lang] = true, true
			}
			if out.AutoDetect.Enabled && len(out.AutoDetect.Languages) < 2 {
				return nil, l10n.Errf("config.parse.autodetect_enabled_languages", "[AutoDetect] \"Enabled\" needs at least two Languages entries; with one there is no layout to correct to")
			}

		case "Actions":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.actions_shape", "[Actions] must consist of \"name = value\" pairs")
			}
			out.Actions.Custom = make(map[string][]string, ActionsCustomHint(len(t)))
			for k, v := range t {
				switch k {
				case "SeqLength":
					n, ok := v.(int64)
					if !ok || n < 1 || n > 255 {
						return nil, l10n.Errf("config.parse.actions_seq_length", "Actions.SeqLength must be integer between 1 and 255")
					}
					out.Actions.SeqLength = int(n)
				case "WordChars":
					s, ok := v.(string)
					if !ok {
						return nil, l10n.Errf("config.parse.actions_wordchars_type", "\"WordChars\" value must be string")
					}
					out.Actions.WordChars = s
				case "WordHead":
					s, ok := v.(string)
					if !ok {
						return nil, l10n.Errf("config.parse.actions_wordhead_type", "\"WordHead\" value must be string")
					}
					out.Actions.WordHead = s
				case "NewWord":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, l10n.Errf("config.parse.actions_newword_type", "\"NewWord\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.NewWord = append(out.Actions.NewWord, seq.(string))
					}
				case "NewSentence":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, l10n.Errf("config.parse.actions_newsentence_type", "\"NewSentence\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.NewSentence = append(out.Actions.NewSentence, seq.(string))
					}
				case "Compose":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, l10n.Errf("config.parse.actions_compose_type", "\"Compose\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.Compose = append(out.Actions.Compose, seq.(string))
					}
				case "TypeClipboard":
					arr, ok := v.([]interface{})
					if !ok {
						return nil, l10n.Errf("config.parse.actions_typeclipboard_type", "\"TypeClipboard\" value must be array of strings")
					}
					for _, seq := range arr {
						out.Actions.TypeClipboard = append(out.Actions.TypeClipboard, seq.(string))
					}
				default:
					if ActionMatchRe.MatchString(k) {
						arr, ok := v.([]interface{})
						if !ok {
							return nil, l10n.Errf("config.parse.actions_action_values", "\"Action.\" values must be arrays of strings")
						}
						name := strings.TrimLeft(ActionNameRe.FindString(k), ".")
						for _, seq := range arr {
							out.Actions.Custom[name] = append(out.Actions.Custom[name], seq.(string))
						}
					} else {
						return nil, l10n.Errf("config.parse.actions_unknown_key", "unknown key \"{{.Key}}\" in [Actions] section", "Key", k)
					}
				}
			}

		case "Action":
			t, ok := value.(map[string]interface{})
			if !ok {
				return nil, l10n.Errf("config.parse.actionset_shape", "[Action.*] must be a map of sections, got {{.Type}}", "Type", value)
			}
			out.ActionSet = make(map[string]TAction, len(t))
			for key, val := range t {
				raw, err := toml.Marshal(val)
				if err != nil {
					return nil, l10n.Errf("config.parse.actionset_entry", "unable to parse [Action.{{.Name}}]:\n{{.Err}}", "Name", key, "Err", err.Error())
				}
				act := TAction{}
				if err = toml.Unmarshal(raw, &act); err != nil {
					return nil, l10n.Errf("config.parse.actionset_entry", "unable to parse [Action.{{.Name}}]:\n{{.Err}}", "Name", key, "Err", err.Error())
				}
				out.ActionSet[key] = act
			}

		case "Keyboard":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.keyboard", "unable to parse [Keyboard]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Keyboard); err != nil {
				return nil, l10n.Errf("config.parse.keyboard", "unable to parse [Keyboard]:\n{{.Err}}", "Err", err.Error())
			}

		case "Wayland":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.wayland", "unable to parse [Wayland]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.Wayland); err != nil {
				return nil, l10n.Errf("config.parse.wayland", "unable to parse [Wayland]:\n{{.Err}}", "Err", err.Error())
			}

		case "GUI":
			raw, err := toml.Marshal(value)
			if err != nil {
				return nil, l10n.Errf("config.parse.gui", "unable to parse [GUI]:\n{{.Err}}", "Err", err.Error())
			}
			if err = toml.Unmarshal(raw, &out.GUI); err != nil {
				return nil, l10n.Errf("config.parse.gui", "unable to parse [GUI]:\n{{.Err}}", "Err", err.Error())
			}
			// An empty Language means "follow the OS locale"; anything else has
			// to name a catalogue l10n actually loaded, or the GUI's picker and
			// the daemon's status answers would silently disagree on the locale.
			if v := strings.TrimSpace(out.GUI.Language); v != "" && !knownLanguage(v) {
				return nil, l10n.Errf("config.parse.gui_language", "[GUI] unknown Language \"{{.Language}}\" (available: {{.Available}})", "Language", v, "Available", strings.Join(languageCodes(), ", "))
			}

		default:
			return nil, l10n.Errf("config.parse.unknown_section", "unknown section name [{{.Section}}]", "Section", key)
		}
	}

	// Verify every custom action has a definition.
	for key := range out.Actions.Custom {
		if _, ok := out.ActionSet[key]; !ok {
			return nil, l10n.Errf("config.parse.action_definition_missing", "action definition not found for \"Action.{{.Name}}\"", "Name", key)
		}
	}

	// A hotkey may only bind to a section that exists.
	for _, hk := range out.Hotkeys {
		if _, ok := out.ActionSet[hk.Action]; !ok {
			return nil, l10n.Errf("config.parse.hotkey_action_missing", "[[Hotkeys]] action definition not found for \"{{.Name}}\"", "Name", hk.Action)
		}
	}

	// Auto-detection decides on the XKB group a word was typed in. Under
	// [Wayland] BypassX the daemon cannot read a group at all — Language() there
	// reports only the switch it made itself — so the section would act on a
	// guess. This cannot live in the case above: the two sections arrive in map
	// iteration order, which is random.
	if out.AutoDetect.Enabled && out.Wayland.BypassX {
		return nil, l10n.Errf("config.parse.autodetect_bypassx", "[AutoDetect] cannot be Enabled together with [Wayland] BypassX = true: the layout is not observable there")
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

// knownLanguage reports whether raw names a locale whose catalogue l10n has
// loaded. It mirrors l10n's internal canonical() rather than calling l10n.Set:
// validating a file must not switch the process-wide locale as a side effect.
// The comparison is on the base code -- lowercased, cut at ".", "@" and the
// first "-" or "_" separator -- so "ru", "RU", "ru-RU", "ru_RU" and "ru.UTF-8"
// all pass exactly when the ru catalogue is embedded.
func knownLanguage(raw string) bool {
	cut := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexAny(cut, ".@"); i >= 0 {
		cut = cut[:i]
	}
	hyphen := strings.ReplaceAll(cut, "_", "-")
	base := strings.SplitN(hyphen, "-", 2)[0]
	for _, d := range l10n.Available() {
		if d.Code == cut || d.Code == hyphen || d.Code == base {
			return true
		}
	}
	return false
}

// languageCodes lists the loaded catalogue codes for the [GUI] validation
// message, in the sorted order l10n.Available() already returns.
func languageCodes() []string {
	avail := l10n.Available()
	codes := make([]string, len(avail))
	for i, d := range avail {
		codes[i] = d.Code
	}
	return codes
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
			return l10n.Errf("config.parse.action_cycle", "\"Action.{{.Name}}\" refers to itself: {{.Chain}}", "Name", name, "Chain", chain)
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
