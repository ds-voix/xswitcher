// Package config — see types.go for overview.
// parse.go extracts the TOML→struct conversion from src/xswitcher.go:config()
// so both the daemon and the GUI can share it without pulling cgo into the GUI.
package config

import (
	"fmt"
	"regexp"
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

	return out, nil
}
