package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// DBusConfig describes a dbus action.
type DBusConfig struct {
	Object string `toml:"object,omitempty"`
	Path   string `toml:"path,omitempty"`
	Method string `toml:"method,omitempty"`
	Value  string `toml:"value,omitempty"`
}

// ActionConfig describes an action that can be triggered.
type ActionConfig struct {
	Deck    string     `toml:"deck,omitempty"`
	Keycode string     `toml:"keycode,omitempty"`
	Exec    string     `toml:"exec,omitempty"`
	Paste   string     `toml:"paste,omitempty"`
	Device  string     `toml:"device,omitempty"`
	DBus    DBusConfig `toml:"dbus,omitempty"`
}

// WidgetConfig describes configuration data for widgets.
type WidgetConfig struct {
	ID       string                 `toml:"id,omitempty"`
	Interval uint                   `toml:"interval,omitempty"`
	Config   map[string]interface{} `toml:"config,omitempty"`
}

// KeyConfig holds the entire configuration for a single key.
type KeyConfig struct {
	Index      uint8         `toml:"index"`
	Widget     WidgetConfig  `toml:"widget"`
	Action     *ActionConfig `toml:"action,omitempty"`
	ActionHold *ActionConfig `toml:"action_hold,omitempty"`

	// Color is the LED color of a touch key (Stream Deck Neo), like
	// "#ff8000".
	Color string `toml:"color,omitempty"`
}

// Keys is a slice of keys.
type Keys []KeyConfig

// DialConfig holds the configuration for a dial (Stream Deck+, + XL and
// Studio) and the touch screen segment above it.
type DialConfig struct {
	Index           uint8         `toml:"index"`
	Widget          WidgetConfig  `toml:"widget,omitempty"`
	Action          *ActionConfig `toml:"action,omitempty"`
	ActionHold      *ActionConfig `toml:"action_hold,omitempty"`
	ActionLeft      *ActionConfig `toml:"action_left,omitempty"`
	ActionRight     *ActionConfig `toml:"action_right,omitempty"`
	ActionTouch     *ActionConfig `toml:"action_touch,omitempty"`
	ActionTouchHold *ActionConfig `toml:"action_touch_hold,omitempty"`

	// RingColor and KnobColor are the colors of the dial's LED ring and
	// knob LED (Stream Deck Studio), like "#ff8000".
	RingColor string `toml:"ring_color,omitempty"`
	KnobColor string `toml:"knob_color,omitempty"`
	// RingValue is a command printing a value from 0 to 100, shown as the
	// lit part of the LED ring. It is run every RingInterval milliseconds
	// and after the dial is used.
	RingValue    string `toml:"ring_value,omitempty"`
	RingInterval uint   `toml:"ring_interval,omitempty"`
}

// Dials is a slice of dials.
type Dials []DialConfig

// SwipeConfig holds the actions for swipes on the touch screen.
type SwipeConfig struct {
	Left  *ActionConfig `toml:"left,omitempty"`
	Right *ActionConfig `toml:"right,omitempty"`
}

// InfoConfig holds the configuration for the info screen (Stream Deck Neo).
type InfoConfig struct {
	Widget WidgetConfig `toml:"widget,omitempty"`
}

// DeckConfig is the central configuration struct.
type DeckConfig struct {
	Background string      `toml:"background,omitempty"`
	Parent     string      `toml:"parent,omitempty"`
	Keys       Keys        `toml:"keys"`
	Dials      Dials       `toml:"dials,omitempty"`
	Swipe      SwipeConfig `toml:"swipe,omitempty"`
	Info       InfoConfig  `toml:"info,omitempty"`
}

// MergeDeckConfig merges key, dial and info screen configuration from multiple
// configs.
func MergeDeckConfig(base, parent *DeckConfig) DeckConfig {
	merged := make(map[byte]KeyConfig)
	for _, config := range parent.Keys {
		merged[config.Index] = config
	}
	for _, config := range base.Keys {
		merged[config.Index] = config
	}

	keys := make(Keys, 0, len(merged))
	for _, config := range merged {
		keys = append(keys, config)
	}

	mergedDials := make(map[byte]DialConfig)
	for _, config := range parent.Dials {
		mergedDials[config.Index] = config
	}
	for _, config := range base.Dials {
		mergedDials[config.Index] = config
	}

	dials := make(Dials, 0, len(mergedDials))
	for _, config := range mergedDials {
		dials = append(dials, config)
	}

	background := base.Background
	if background == "" {
		background = parent.Background
	}

	swipe := base.Swipe
	if swipe.Left == nil {
		swipe.Left = parent.Swipe.Left
	}
	if swipe.Right == nil {
		swipe.Right = parent.Swipe.Right
	}

	info := base.Info
	if info.Widget.ID == "" {
		info = parent.Info
	}

	return DeckConfig{
		Background: background,
		Parent:     base.Parent,
		Keys:       keys,
		Dials:      dials,
		Swipe:      swipe,
		Info:       info,
	}
}

// LoadConfigFromFile loads a DeckConfig from a file while checking for circular
// dependencies.
func LoadConfigFromFile(base, path string, files []string) (DeckConfig, error) {
	config := DeckConfig{}

	filename, err := expandPath(base, path)
	if err != nil {
		return config, err
	}

	// check for circular dependencies
	for _, prev := range files {
		// TODO: improve error message with actual file names
		if prev == filename {
			return config, errors.New("circular reference")
		}
	}

	file, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}

	_, err = toml.Decode(string(file), &config)
	if config.Parent != "" {
		parent, err := LoadConfigFromFile(base, config.Parent, append(files, filename))
		if err != nil {
			return parent, err
		}

		merged := MergeDeckConfig(&config, &parent)
		return merged, err
	}

	return config, err
}

// LoadConfig loads config from filename.
func LoadConfig(path string) (DeckConfig, error) {
	base := filepath.Dir(path)
	filename := filepath.Base(path)

	return LoadConfigFromFile(base, filename, []string{})
}

// Save writes config as json to filename.
func (c DeckConfig) Save(filename string) error {
	var b bytes.Buffer
	e := toml.NewEncoder(&b)
	if err := e.Encode(c); err != nil {
		return err
	}

	return os.WriteFile(filename, b.Bytes(), 0600)
}

// ConfigValue tries to convert an interface{} to the desired type. A nil
// value (an option that isn't set) leaves dst unchanged.
func ConfigValue(v interface{}, dst interface{}) error {
	if v == nil {
		return nil
	}

	switch d := dst.(type) {
	case *string:
		switch vt := v.(type) {
		case string:
			*d = vt
		default:
			return fmt.Errorf("expected a string, got %v", reflect.TypeOf(vt))
		}

	case *bool:
		switch vt := v.(type) {
		case bool:
			*d = vt
		case string:
			b, err := strconv.ParseBool(vt)
			if err != nil {
				return fmt.Errorf("expected true or false, got %q", vt)
			}
			*d = b
		case int64:
			*d = vt > 0
		default:
			return fmt.Errorf("expected true or false, got %v", reflect.TypeOf(vt))
		}

	case *int64:
		switch vt := v.(type) {
		case int64:
			*d = vt
		case float64:
			*d = int64(vt)
		case string:
			x, err := strconv.ParseInt(vt, 0, 64)
			if err != nil {
				return fmt.Errorf("expected an integer, got %q", vt)
			}
			*d = x
		default:
			return fmt.Errorf("expected an integer, got %v", reflect.TypeOf(vt))
		}

	case *float64:
		switch vt := v.(type) {
		case int64:
			*d = float64(vt)
		case float64:
			*d = vt
		case string:
			x, err := strconv.ParseFloat(vt, 64)
			if err != nil {
				return fmt.Errorf("expected a number, got %q", vt)
			}
			*d = x
		default:
			return fmt.Errorf("expected a number, got %v", reflect.TypeOf(vt))
		}

	case *color.Color:
		switch vt := v.(type) {
		case string:
			x, err := parseColor(vt)
			if err != nil {
				return err
			}
			*d = x
		default:
			return fmt.Errorf("expected a color like \"#ff8000\", got %v", reflect.TypeOf(vt))
		}

	case *[]string:
		switch vt := v.(type) {
		case string:
			*d = strings.Split(vt, ";")
		default:
			return fmt.Errorf("expected a string, got %v", reflect.TypeOf(vt))
		}

	case *[]color.Color:
		switch vt := v.(type) {
		case string:
			var clrs []color.Color
			for _, cl := range strings.Split(vt, ";") {
				clr, err := parseColor(cl)
				if err != nil {
					return err
				}
				clrs = append(clrs, clr)
			}
			*d = clrs
		default:
			return fmt.Errorf("expected colors like \"#ff8000;#ffffff\", got %v", reflect.TypeOf(vt))
		}

	default:
		return fmt.Errorf("unhandled dst type %+v", reflect.TypeOf(dst))
	}

	return nil
}

// configReader reads widget options, remembering the first error.
type configReader struct {
	config map[string]interface{}
	err    error
}

// read reads the option with the given name into dst, if it is set.
func (r *configReader) read(name string, dst interface{}) {
	if r.err != nil {
		return
	}
	if err := ConfigValue(r.config[name], dst); err != nil {
		r.err = fmt.Errorf("option %s: %w", name, err)
	}
}
