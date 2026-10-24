//go:build linux
// +build linux

package main

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// keymapUS is the keymap the virtual keyboard announces. Keycode actions name
// physical keys, so a fixed layout keeps them independent of the user's.
// Generated with: xkbcli compile-keymap --rules evdev --model pc105 --layout us
//
//go:embed assets/keymap/us.xkb
var keymapUS []byte

// Wayland object IDs and opcodes used by the virtual keyboard client. Only the
// handful of requests and events it needs are covered.
const (
	wlDisplayID = 1

	wlDisplaySync        = 0
	wlDisplayGetRegistry = 1
	wlDisplayError       = 0

	wlRegistryBind   = 0
	wlRegistryGlobal = 0

	wlCallbackDone = 0

	vkbdManagerCreate = 0

	vkbdKeymap    = 0
	vkbdKey       = 1
	vkbdModifiers = 2
	vkbdDestroy   = 3

	vkbdKeymapFormatXKBv1 = 1
	vkbdKeyReleased       = 0
	vkbdKeyPressed        = 1

	wlHeaderSize = 8
)

// XKB modifier masks of the real modifiers in keymapUS.
const (
	modShift   = 1 << 0
	modControl = 1 << 2
	modMod1    = 1 << 3 // Alt
	modMod4    = 1 << 6 // Super
)

// modifierMasks maps evdev keycodes of modifier keys to their XKB mask.
var modifierMasks = map[int]uint32{
	42:  modShift,   // LeftShift
	54:  modShift,   // RightShift
	29:  modControl, // LeftCtrl
	97:  modControl, // RightCtrl
	56:  modMod1,    // LeftAlt
	100: modMod1,    // RightAlt
	125: modMod4,    // LeftMeta
	126: modMod4,    // RightMeta
}

// waylandKeyboard emulates key presses with the zwp_virtual_keyboard_v1
// protocol. Events only reach the compositor session it is connected to.
type waylandKeyboard struct {
	mu      sync.Mutex
	conn    *net.UnixConn
	rbuf    []byte
	nextID  uint32
	vkbd    uint32
	start   time.Time
	pressed map[int]bool
}

// waylandSocket returns the path of the compositor's socket.
func waylandSocket() (string, error) {
	display := os.Getenv("WAYLAND_DISPLAY")
	if display == "" {
		return "", errors.New("WAYLAND_DISPLAY is not set")
	}
	if filepath.IsAbs(display) {
		return display, nil
	}

	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set")
	}
	return filepath.Join(runtime, display), nil
}

func newWaylandKeyboard() (*waylandKeyboard, error) {
	path, err := waylandSocket()
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("can't connect to the compositor: %w", err)
	}

	k := &waylandKeyboard{
		conn:    conn,
		nextID:  wlDisplayID + 1,
		start:   time.Now(),
		pressed: map[int]bool{},
	}
	if err := k.setup(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return k, nil
}

func (k *waylandKeyboard) newID() uint32 {
	id := k.nextID
	k.nextID++
	return id
}

// setup creates the virtual keyboard and uploads its keymap.
func (k *waylandKeyboard) setup() error {
	registry := k.newID()
	if err := k.send(wlMessage(wlDisplayID, wlDisplayGetRegistry, registry), nil); err != nil {
		return err
	}

	type global struct {
		name    uint32
		version uint32
	}
	globals := map[string]global{}
	err := k.roundtrip(func(obj uint32, op uint16, body []byte) {
		if obj != registry || op != wlRegistryGlobal {
			return
		}
		name, iface, version, ok := parseGlobal(body)
		if !ok {
			return
		}
		if _, seen := globals[iface]; !seen {
			globals[iface] = global{name, version}
		}
	})
	if err != nil {
		return err
	}

	seat, ok := globals["wl_seat"]
	if !ok {
		return errors.New("compositor has no wl_seat")
	}
	manager, ok := globals["zwp_virtual_keyboard_manager_v1"]
	if !ok {
		return errors.New("compositor doesn't support the virtual keyboard protocol (zwp_virtual_keyboard_manager_v1)")
	}

	seatID, managerID, vkbdID := k.newID(), k.newID(), k.newID()
	msgs := [][]byte{
		wlMessage(registry, wlRegistryBind, seat.name, "wl_seat", uint32(1), seatID),
		wlMessage(registry, wlRegistryBind, manager.name, "zwp_virtual_keyboard_manager_v1", uint32(1), managerID),
		wlMessage(managerID, vkbdManagerCreate, seatID, vkbdID),
	}
	for _, m := range msgs {
		if err := k.send(m, nil); err != nil {
			return err
		}
	}

	keymap, err := keymapFile(keymapUS)
	if err != nil {
		return err
	}
	defer keymap.Close() //nolint:errcheck
	//nolint:gosec // the keymap is a few KB
	size := uint32(len(keymapUS) + 1)
	if err := k.send(wlMessage(vkbdID, vkbdKeymap, uint32(vkbdKeymapFormatXKBv1), size), syscall.UnixRights(int(keymap.Fd()))); err != nil {
		return err
	}
	k.vkbd = vkbdID

	// surfaces protocol errors, e.g. when the compositor refuses the client
	return k.roundtrip(nil)
}

// keymapFile returns a memfd holding the NUL-terminated keymap.
func keymapFile(keymap []byte) (*os.File, error) {
	fd, err := unix.MemfdCreate("deckmaster-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("can't create keymap file: %w", err)
	}
	f := os.NewFile(uintptr(fd), "deckmaster-keymap")
	if _, err := f.Write(append(keymap[:len(keymap):len(keymap)], 0)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("can't write keymap: %w", err)
	}
	return f, nil
}

func (k *waylandKeyboard) send(msg, oob []byte) error {
	n, _, err := k.conn.WriteMsgUnix(msg, oob, nil)
	if err != nil {
		return fmt.Errorf("writing to the compositor failed: %w", err)
	}
	if n != len(msg) {
		return errors.New("short write to the compositor")
	}
	return nil
}

// roundtrip waits until the compositor has handled all requests sent so far,
// passing every event received meanwhile to handle.
func (k *waylandKeyboard) roundtrip(handle func(obj uint32, op uint16, body []byte)) error {
	callback := k.newID()
	if err := k.send(wlMessage(wlDisplayID, wlDisplaySync, callback), nil); err != nil {
		return err
	}

	if err := k.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("can't set read deadline: %w", err)
	}
	for {
		obj, op, body, err := k.readEvent()
		if err != nil {
			return err
		}
		switch {
		case obj == wlDisplayID && op == wlDisplayError:
			return parseDisplayError(body)
		case obj == callback && op == wlCallbackDone:
			return nil
		case handle != nil:
			handle(obj, op, body)
		}
	}
}

// readEvent reads the next event from the compositor.
func (k *waylandKeyboard) readEvent() (uint32, uint16, []byte, error) {
	for {
		if len(k.rbuf) >= wlHeaderSize {
			obj := binary.NativeEndian.Uint32(k.rbuf)
			word := binary.NativeEndian.Uint32(k.rbuf[4:])
			size := int(word >> 16)
			if size < wlHeaderSize {
				return 0, 0, nil, errors.New("malformed message from the compositor")
			}
			if len(k.rbuf) >= size {
				body := k.rbuf[wlHeaderSize:size]
				k.rbuf = k.rbuf[size:]
				return obj, uint16(word & 0xffff), body, nil //nolint:gosec // masked
			}
		}

		buf := make([]byte, 4096)
		oob := make([]byte, unix.CmsgSpace(4*28))
		n, oobn, _, _, err := k.conn.ReadMsgUnix(buf, oob)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("reading from the compositor failed: %w", err)
		}
		closeReceivedFDs(oob[:oobn])
		k.rbuf = append(k.rbuf, buf[:n]...)
	}
}

// closeReceivedFDs closes file descriptors the compositor sent along, none
// of the events handled here carry one.
func closeReceivedFDs(oob []byte) {
	msgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, m := range msgs {
		fds, err := syscall.ParseUnixRights(&m)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			_ = syscall.Close(fd)
		}
	}
}

func (k *waylandKeyboard) key(key int, state uint32) error {
	if key < 0 {
		return fmt.Errorf("invalid keycode %d", key)
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	//nolint:gosec // wraps after 49 days, like any Wayland timestamp
	t := uint32(time.Since(k.start).Milliseconds())
	//nolint:gosec // key is not negative
	if err := k.send(wlMessage(k.vkbd, vkbdKey, t, uint32(key), state), nil); err != nil {
		return err
	}

	// the protocol doesn't derive modifiers from key events, send them along
	if _, ok := modifierMasks[key]; !ok {
		return nil
	}
	if state == vkbdKeyPressed {
		k.pressed[key] = true
	} else {
		delete(k.pressed, key)
	}
	return k.send(wlMessage(k.vkbd, vkbdModifiers, modifierMask(k.pressed), uint32(0), uint32(0), uint32(0)), nil)
}

// modifierMask returns the XKB modifier mask of the pressed keys.
func modifierMask(pressed map[int]bool) uint32 {
	var mask uint32
	for key := range pressed {
		mask |= modifierMasks[key]
	}
	return mask
}

func (k *waylandKeyboard) KeyDown(key int) error {
	return k.key(key, vkbdKeyPressed)
}

func (k *waylandKeyboard) KeyUp(key int) error {
	return k.key(key, vkbdKeyReleased)
}

func (k *waylandKeyboard) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	err := k.send(wlMessage(k.vkbd, vkbdDestroy), nil)
	if err == nil {
		err = k.roundtrip(nil)
	}
	return errors.Join(err, k.conn.Close())
}

// wlMessage encodes a Wayland request. Arguments are uint32s (uint, int,
// object and new_id) or strings.
func wlMessage(obj uint32, opcode uint16, args ...interface{}) []byte {
	buf := make([]byte, wlHeaderSize, 32)
	for _, a := range args {
		switch v := a.(type) {
		case uint32:
			buf = binary.NativeEndian.AppendUint32(buf, v)
		case string:
			//nolint:gosec // interface names are short
			buf = binary.NativeEndian.AppendUint32(buf, uint32(len(v)+1))
			buf = append(buf, v...)
			buf = append(buf, 0)
			for len(buf)%4 != 0 {
				buf = append(buf, 0)
			}
		default:
			panic(fmt.Sprintf("wlMessage: unsupported argument type %T", a))
		}
	}

	binary.NativeEndian.PutUint32(buf, obj)
	//nolint:gosec // requests are tiny
	binary.NativeEndian.PutUint32(buf[4:], uint32(len(buf))<<16|uint32(opcode))
	return buf
}

// wlString decodes a string argument, returning it and the remaining body.
func wlString(body []byte) (string, []byte, bool) {
	if len(body) < 4 {
		return "", nil, false
	}
	n := int(binary.NativeEndian.Uint32(body))
	padded := (n + 3) &^ 3
	if n == 0 || len(body) < 4+padded {
		return "", nil, false
	}
	return string(body[4 : 4+n-1]), body[4+padded:], true
}

// parseGlobal decodes a wl_registry.global event.
func parseGlobal(body []byte) (uint32, string, uint32, bool) {
	if len(body) < 4 {
		return 0, "", 0, false
	}
	name := binary.NativeEndian.Uint32(body)
	iface, rest, ok := wlString(body[4:])
	if !ok || len(rest) < 4 {
		return 0, "", 0, false
	}
	return name, iface, binary.NativeEndian.Uint32(rest), true
}

// parseDisplayError turns a wl_display.error event into an error.
func parseDisplayError(body []byte) error {
	if len(body) < 8 {
		return errors.New("compositor reported a protocol error")
	}
	obj := binary.NativeEndian.Uint32(body)
	code := binary.NativeEndian.Uint32(body[4:])
	msg, _, _ := wlString(body[8:])
	return fmt.Errorf("compositor reported a protocol error on object %d (code %d): %s", obj, code, msg)
}
