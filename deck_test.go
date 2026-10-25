package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestParseDBusValue(t *testing.T) {
	tests := []struct {
		value string
		want  []interface{}
	}{
		{"", nil},
		{"hello", []interface{}{"hello"}},
		{"http://example.com", []interface{}{"http://example.com"}},
		{"string:int32:4", []interface{}{"int32:4"}},
		{"int32:-4", []interface{}{int32(-4)}},
		{"uint32:4", []interface{}{uint32(4)}},
		{"int64:4", []interface{}{int64(4)}},
		{"uint64:4", []interface{}{uint64(4)}},
		{"int16:4", []interface{}{int16(4)}},
		{"uint16:4", []interface{}{uint16(4)}},
		{"byte:255", []interface{}{byte(255)}},
		{"double:0.5", []interface{}{0.5}},
		{"boolean:true", []interface{}{true}},
		{"objpath:/org/example", []interface{}{dbus.ObjectPath("/org/example")}},
	}
	for _, tt := range tests {
		got, err := parseDBusValue(tt.value)
		if err != nil {
			t.Errorf("parseDBusValue(%q): %v", tt.value, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseDBusValue(%q) = %#v, want %#v", tt.value, got, tt.want)
		}
	}

	for _, value := range []string{"int32:x", "byte:256", "boolean:maybe"} {
		if _, err := parseDBusValue(value); err == nil {
			t.Errorf("parseDBusValue(%q): expected an error", value)
		}
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"pavucontrol", []string{"pavucontrol"}},
		{"some_command --with-parameters", []string{"some_command", "--with-parameters"}},
		{"a  b\tc", []string{"a", "b", "c"}},
		{`notify-send 'hello world'`, []string{"notify-send", "hello world"}},
		{`notify-send "hello world" now`, []string{"notify-send", "hello world", "now"}},
		{`echo "it's" 'say "hi"'`, []string{"echo", "it's", `say "hi"`}},
		{`echo a\ b`, []string{"echo", "a b"}},
		{`echo "a \"b\""`, []string{"echo", `a "b"`}},
		{`echo 'a\b'`, []string{"echo", `a\b`}},
		{`echo ""`, []string{"echo", ""}},
		{`echo --opt="a b"`, []string{"echo", "--opt=a b"}},
	}
	for _, tt := range tests {
		got, err := splitCommand(tt.cmd)
		if err != nil {
			t.Errorf("splitCommand(%q): %v", tt.cmd, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}

	for _, cmd := range []string{`echo 'a`, `echo "a`, `echo a\`} {
		if _, err := splitCommand(cmd); err == nil {
			t.Errorf("splitCommand(%q): expected an error", cmd)
		}
	}
}

// fakeKeyboard records key events as "+key" and "-key".
type fakeKeyboard struct{ events []string }

func (k *fakeKeyboard) KeyDown(key int) error {
	k.events = append(k.events, fmt.Sprintf("+%d", key))
	return nil
}

func (k *fakeKeyboard) KeyUp(key int) error {
	k.events = append(k.events, fmt.Sprintf("-%d", key))
	return nil
}

func (k *fakeKeyboard) Close() error { return nil }

func withFakeKeyboard(t *testing.T) *fakeKeyboard {
	t.Helper()
	kb := &fakeKeyboard{}
	old := keyboard
	keyboard = kb
	t.Cleanup(func() { keyboard = old })
	return kb
}

func TestEmulateKeyPresses(t *testing.T) {
	tests := []struct {
		keys string
		want []string
	}{
		{"30", []string{"+30", "-30"}},
		// modifiers are held until the last key is released
		{"29-42-47", []string{"+29", "+42", "+47", "-47", "-42", "-29"}},
		{"30/31+1", []string{"+30", "-30", "+31", "-31"}},
		// nothing is pressed when a key in a combination is invalid
		{"29-nosuchkey", nil},
	}
	for _, tt := range tests {
		kb := withFakeKeyboard(t)
		emulateKeyPresses(tt.keys)
		if !reflect.DeepEqual(kb.events, tt.want) {
			t.Errorf("emulateKeyPresses(%q) = %v, want %v", tt.keys, kb.events, tt.want)
		}
	}
}

func TestTurnDial(t *testing.T) {
	d := &Deck{Dials: map[uint8]DialConfig{
		0: {
			ActionLeft:  &ActionConfig{Keycode: "1"},
			ActionRight: &ActionConfig{Keycode: "2"},
		},
	}}
	tests := []struct {
		index uint8
		steps int8
		want  int
		key   string
	}{
		{0, 3, 3, "+2"},
		{0, -2, 2, "+1"},
		{0, 100, maxDialSteps, "+2"},
		{1, 3, 0, ""}, // no such dial
	}
	for _, tt := range tests {
		kb := withFakeKeyboard(t)
		d.turnDial(nil, tt.index, tt.steps)
		if got := len(kb.events) / 2; got != tt.want {
			t.Errorf("turnDial(%d, %d): %d presses, want %d", tt.index, tt.steps, got, tt.want)
		}
		if tt.want > 0 && kb.events[0] != tt.key {
			t.Errorf("turnDial(%d, %d): pressed %s, want %s", tt.index, tt.steps, kb.events[0], tt.key)
		}
	}
}

func TestEmulateClipboard(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "wl-copy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")

	tests := []struct {
		keys string
		want []string
	}{
		{"", []string{"+29", "+47", "-47", "-29"}},
		{"Leftctrl-Leftshift-V", []string{"+29", "+42", "+47", "-47", "-42", "-29"}},
	}
	for _, tt := range tests {
		kb := withFakeKeyboard(t)
		emulateClipboard("hello", tt.keys)
		if !reflect.DeepEqual(kb.events, tt.want) {
			t.Errorf("emulateClipboard(%q) pressed %v, want %v", tt.keys, kb.events, tt.want)
		}
	}

	// nothing is pasted when copying fails
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	kb := withFakeKeyboard(t)
	emulateClipboard("hello", "")
	if len(kb.events) != 0 {
		t.Errorf("pressed %v after copying failed, want nothing", kb.events)
	}
}
