package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bendahl/uinput"
)

// keyInjector emulates key presses. Keys are Linux evdev keycodes.
type keyInjector interface {
	KeyDown(key int) error
	KeyUp(key int) error
	Close() error
}

// newKeyInjector creates the keyboard emulation backend for mode, which is one
// of auto, wayland, x11, uinput or none. Auto prefers the Wayland virtual
// keyboard protocol, then X11's XTEST extension. Both only reach the desktop
// session, unlike uinput, which injects into the kernel and needs write access
// to /dev/uinput, so uinput is never picked automatically.
func newKeyInjector(mode string) (keyInjector, string, error) {
	switch mode {
	case "auto":
		wl, wlErr := newWaylandKeyboard()
		if wlErr == nil {
			return wl, "wayland", nil
		}
		x, xErr := newXTestKeyboard(xorg)
		if xErr == nil {
			return x, "x11", nil
		}
		return nil, "", errors.Join(fmt.Errorf("wayland: %w", wlErr), fmt.Errorf("x11: %w", xErr))
	case "wayland":
		kb, err := newWaylandKeyboard()
		return kb, mode, err
	case "x11":
		kb, err := newXTestKeyboard(xorg)
		return kb, mode, err
	case "uinput":
		kb, err := uinput.CreateKeyboard("/dev/uinput", []byte("Deckmaster"))
		if err != nil {
			return nil, mode, fmt.Errorf("can't create virtual input device (/dev/uinput): %w", err)
		}
		return kb, mode, nil
	case "none":
		return nil, mode, errors.New("disabled with -keyboard none")
	default:
		return nil, mode, fmt.Errorf("unknown keyboard backend %q (want auto, wayland, x11, uinput or none)", mode)
	}
}

// initKeyboard sets up keyboard emulation. It returns a function that closes
// the backend.
func initKeyboard(mode string) func() {
	kb, name, err := newKeyInjector(mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not set up keyboard emulation: %s\n", err)
		fmt.Fprintln(os.Stderr, "Emulating keyboard events will be disabled!")
		return func() {}
	}

	verbosef("Emulating keyboard events via %s", name)
	keyboard = kb
	return func() {
		if err := kb.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Closing the virtual keyboard failed: %s\n", err)
		}
	}
}
