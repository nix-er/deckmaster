//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestWlMessage(t *testing.T) {
	msg := wlMessage(2, wlRegistryBind, uint32(7), "wl_seat", uint32(1), uint32(4))

	// header + name + (len + "wl_seat\0") + version + id
	want := 8 + 4 + (4 + 8) + 4 + 4
	if len(msg) != want {
		t.Fatalf("message length = %d, want %d", len(msg), want)
	}
	if obj := binary.NativeEndian.Uint32(msg); obj != 2 {
		t.Errorf("object = %d, want 2", obj)
	}
	word := binary.NativeEndian.Uint32(msg[4:])
	if size := int(word >> 16); size != want {
		t.Errorf("size = %d, want %d", size, want)
	}
	if op := word & 0xffff; op != wlRegistryBind {
		t.Errorf("opcode = %d, want %d", op, wlRegistryBind)
	}
	if !bytes.Equal(msg[16:24], []byte("wl_seat\x00")) {
		t.Errorf("string = %q, want %q", msg[16:24], "wl_seat\x00")
	}
}

func TestWlMessagePadding(t *testing.T) {
	// "abcd\0" is five bytes and has to be padded to eight
	msg := wlMessage(1, 0, "abcd")
	if len(msg) != 8+4+8 {
		t.Fatalf("message length = %d, want %d", len(msg), 8+4+8)
	}
	if !bytes.Equal(msg[12:], []byte("abcd\x00\x00\x00\x00")) {
		t.Errorf("padded string = %q", msg[12:])
	}
}

func TestParseGlobal(t *testing.T) {
	// an event body has the same layout as a request body
	body := wlMessage(0, 0, uint32(42), "zwp_virtual_keyboard_manager_v1", uint32(1))[wlHeaderSize:]
	name, iface, version, ok := parseGlobal(body)
	if !ok {
		t.Fatal("parseGlobal failed")
	}
	if name != 42 || iface != "zwp_virtual_keyboard_manager_v1" || version != 1 {
		t.Errorf("got (%d, %q, %d)", name, iface, version)
	}

	if _, _, _, ok := parseGlobal(body[:10]); ok {
		t.Error("parseGlobal accepted a truncated event")
	}
}

func TestParseDisplayError(t *testing.T) {
	body := wlMessage(0, 0, uint32(5), uint32(0), "unauthorized")[wlHeaderSize:]
	err := parseDisplayError(body)
	want := "compositor reported a protocol error on object 5 (code 0): unauthorized"
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}

func TestModifierMask(t *testing.T) {
	tests := []struct {
		pressed []int
		want    uint32
	}{
		{nil, 0},
		{[]int{29}, modControl},
		{[]int{29, 42}, modControl | modShift},
		{[]int{125, 56, 97}, modMod4 | modMod1 | modControl},
		{[]int{30}, 0}, // A is no modifier
	}
	for _, tt := range tests {
		pressed := map[int]bool{}
		for _, k := range tt.pressed {
			pressed[k] = true
		}
		if got := modifierMask(pressed); got != tt.want {
			t.Errorf("modifierMask(%v) = %#x, want %#x", tt.pressed, got, tt.want)
		}
	}
}

func TestFindWaylandSocket(t *testing.T) {
	dir := t.TempDir()
	if _, ok := findWaylandSocket(dir); ok {
		t.Error("found a socket in an empty directory")
	}

	for _, name := range []string{"wayland-2", "wayland-1", "niri.wayland-1.sock"} {
		l, err := net.Listen("unix", filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close() //nolint:errcheck
	}
	// lock files are no sockets
	if err := os.WriteFile(filepath.Join(dir, "wayland-0.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := findWaylandSocket(dir)
	if want := filepath.Join(dir, "wayland-1"); !ok || got != want {
		t.Errorf("got %q, %v, want %q", got, ok, want)
	}
}

// TestWaylandKeyboard creates and destroys a virtual keyboard on the running
// compositor without sending any keys. Run it inside a Wayland session with
// DECKMASTER_WAYLAND_TEST=1.
func TestWaylandKeyboard(t *testing.T) {
	if os.Getenv("DECKMASTER_WAYLAND_TEST") == "" {
		t.Skip("set DECKMASTER_WAYLAND_TEST=1 to test against the running compositor")
	}

	kb, err := newWaylandKeyboard()
	if err != nil {
		t.Fatal(err)
	}
	if err := kb.Close(); err != nil {
		t.Fatal(err)
	}
}
