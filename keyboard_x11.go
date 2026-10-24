//go:build linux
// +build linux

package main

import (
	"errors"
	"fmt"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// X11 keycodes are evdev keycodes shifted by 8.
const xKeycodeOffset = 8

// xtestKeyboard emulates key presses with X11's XTEST extension. Any X client
// can already do that, so it needs no extra permissions.
type xtestKeyboard struct {
	x *Xorg
}

func newXTestKeyboard(x *Xorg) (*xtestKeyboard, error) {
	if x == nil {
		return nil, errors.New("not connected to an X server")
	}
	if err := xtest.Init(x.conn); err != nil {
		return nil, fmt.Errorf("XTEST extension unavailable: %w", err)
	}

	return &xtestKeyboard{x: x}, nil
}

func (k *xtestKeyboard) fake(event byte, key int) error {
	code := key + xKeycodeOffset
	if key < 0 || code > 255 {
		return fmt.Errorf("keycode %d out of range for X11", key)
	}

	//nolint:gosec // range checked above
	return xtest.FakeInputChecked(k.x.conn, event, byte(code), 0, k.x.root, 0, 0, 0).Check()
}

func (k *xtestKeyboard) KeyDown(key int) error {
	return k.fake(xproto.KeyPress, key)
}

func (k *xtestKeyboard) KeyUp(key int) error {
	return k.fake(xproto.KeyRelease, key)
}

// Close is a no-op, the X connection is shared and closed elsewhere.
func (k *xtestKeyboard) Close() error {
	return nil
}
