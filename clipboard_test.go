package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeTool installs a script called name in dir that saves its arguments,
// WAYLAND_DISPLAY and stdin next to itself.
func fakeTool(t *testing.T, dir, name string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"echo \"$@\" > \"$(dirname \"$0\")/args\"\n" +
		"echo \"$WAYLAND_DISPLAY\" > \"$(dirname \"$0\")/display\"\n" +
		"cat > \"$(dirname \"$0\")/stdin\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil { //nolint:gosec
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCopyToClipboardWayland(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "wl-copy")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/test")
	t.Setenv("WAYLAND_DISPLAY", "wayland-1")

	if err := copyToClipboard("hello"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(bin, "stdin")); got != "hello" {
		t.Errorf("stdin = %q, want %q", got, "hello")
	}
	// wl-copy gets the socket path, which also works without the variable
	if got, want := readFile(t, filepath.Join(bin, "display")), "/run/user/test/wayland-1\n"; got != want {
		t.Errorf("WAYLAND_DISPLAY = %q, want %q", got, want)
	}
}

func TestCopyToClipboardX11(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "xclip")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no wayland-N socket
	t.Setenv("WAYLAND_DISPLAY", "")

	if err := copyToClipboard("hello"); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, filepath.Join(bin, "args")), "-in -selection clipboard\n"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got := readFile(t, filepath.Join(bin, "stdin")); got != "hello" {
		t.Errorf("stdin = %q, want %q", got, "hello")
	}
}

func TestCopyToClipboardNoTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")

	if err := copyToClipboard("hello"); err == nil {
		t.Error("copyToClipboard succeeded without any clipboard tool")
	}
}
