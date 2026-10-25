package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// copyToClipboard puts text on the clipboard, with wl-copy on Wayland and
// xclip or xsel on X11. Like the Wayland keyboard, it finds the compositor
// without WAYLAND_DISPLAY, e.g. in a systemd service.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	if socket, err := waylandSocket(); err == nil {
		cmd = exec.Command("wl-copy")
		cmd.Env = append(os.Environ(), "WAYLAND_DISPLAY="+socket)
	} else if _, err := exec.LookPath("xclip"); err == nil {
		cmd = exec.Command("xclip", "-in", "-selection", "clipboard")
	} else if _, err := exec.LookPath("xsel"); err == nil {
		cmd = exec.Command("xsel", "--input", "--clipboard")
	} else {
		return errors.New("no Wayland compositor found, and neither xclip nor xsel is installed")
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
