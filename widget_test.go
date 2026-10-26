package main

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/nix-er/streamdeck"
)

func xlDevice() *streamdeck.Device {
	return &streamdeck.Device{
		Columns: 8, Rows: 4, Keys: 32, KeyWidth: 96, KeyHeight: 96, Pixels: 96, DPI: 166, Padding: 32,
	}
}

func plusDevice() *streamdeck.Device {
	return &streamdeck.Device{
		Columns: 4, Rows: 2, Keys: 8, KeyWidth: 120, KeyHeight: 120, Pixels: 120, DPI: 166, Padding: 16,
		Dials: 4, ScreenWidth: 800, ScreenHeight: 100, ScreenSegments: 4, ScreenDPI: 166,
	}
}

// All example decks must load, now that invalid options are errors.
func TestLoadExampleDecks(t *testing.T) {
	devices := map[string]*streamdeck.Device{
		"main.deck":       xlDevice(),
		"plus.deck":       plusDevice(),
		"plus-media.deck": plusDevice(),
		"neo.deck":        neoDevice(),
		"studio.deck":     studioDevice(),
	}
	decks, err := filepath.Glob("decks/*.deck")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range decks {
		dev, ok := devices[filepath.Base(path)]
		if !ok {
			t.Errorf("%s: no device to test it with", path)
			continue
		}
		if _, err := LoadDeck(dev, ".", path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestConfigValue(t *testing.T) {
	s := "keep"
	if err := ConfigValue(nil, &s); err != nil || s != "keep" {
		t.Errorf("unset option: got %q, %v; want it unchanged", s, err)
	}

	var c color.Color
	if err := ConfigValue("#ff0000", &c); err != nil || c != (colorful.Color{R: 1}) {
		t.Errorf("color: got %v, %v", c, err)
	}
	var b bool
	if err := ConfigValue("true", &b); err != nil || !b {
		t.Errorf("bool: got %v, %v", b, err)
	}

	for _, tt := range []struct {
		v   interface{}
		dst interface{}
	}{
		{"red", new(color.Color)},
		{"#ff0000;nope", new([]color.Color)},
		{"yes please", new(bool)},
		{"ten", new(int64)},
		{"1.2.3", new(float64)},
		{int64(4), new(string)},
	} {
		if err := ConfigValue(tt.v, tt.dst); err == nil {
			t.Errorf("ConfigValue(%v, %T): expected an error", tt.v, tt.dst)
		}
	}
}

// writeDeck writes a deck config to a temporary file.
func writeDeck(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.deck")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInvalidWidgetOption(t *testing.T) {
	path := writeDeck(t, `
[[keys]]
  index = 3
  [keys.widget]
    id = "button"
    [keys.widget.config]
      color = "#12345"
`)
	_, err := LoadDeck(xlDevice(), ".", path)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"key 3", "button widget", "option color"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q doesn't mention %q", err, want)
		}
	}
}

// A button whose icon can't be loaded keeps its label instead of failing the
// deck.
func TestButtonBrokenIcon(t *testing.T) {
	path := writeDeck(t, `
[[keys]]
  index = 0
  [keys.widget]
    id = "button"
    [keys.widget.config]
      icon = "/nonexistent/icon.png"
      label = "Files"
`)
	deck, err := LoadDeck(xlDevice(), ".", path)
	if err != nil {
		t.Fatal(err)
	}
	w, ok := deck.Widgets[0].(*ButtonWidget)
	if !ok {
		t.Fatalf("key 0 is %T, want a button", deck.Widgets[0])
	}
	if w.icon != nil || w.label != "Files" {
		t.Errorf("got icon %v, label %q; want no icon and the label", w.icon, w.label)
	}
}

// Configuring a clock's color must not drop its default format and fonts,
// and vice versa.
func TestClockOptions(t *testing.T) {
	path := writeDeck(t, `
[[keys]]
  index = 0
  [keys.widget]
    id = "clock"
    [keys.widget.config]
      color = "#ff0000"
`)
	d, err := LoadDeck(xlDevice(), ".", path)
	if err != nil {
		t.Fatal(err)
	}
	clock := d.Widgets[0].(*TimeWidget)
	if want := []string{"%H", "%i", "%s"}; !reflect.DeepEqual(clock.formats, want) {
		t.Errorf("formats = %q, want %q", clock.formats, want)
	}
	if want := []string{"bold", "regular", "thin"}; !reflect.DeepEqual(clock.fonts, want) {
		t.Errorf("fonts = %q, want %q", clock.fonts, want)
	}
	if clock.colors[0] != (colorful.Color{R: 1}) {
		t.Errorf("color = %v, want red", clock.colors[0])
	}
}

// waitForFresh waits until the runner has new output.
func waitForFresh(t *testing.T, r *commandRunner) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !r.Fresh() {
		if time.Now().After(deadline) {
			t.Fatal("commands didn't finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCommandRunner(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "fail")
	r := newCommandRunner("key 0", []string{
		"echo one",
		"test -e " + flag + " && exit 1; echo two",
	})

	// the commands run in the background, there's no output yet
	if got := r.Outputs(); !reflect.DeepEqual(got, []string{"", ""}) {
		t.Errorf("first outputs = %q, want empty", got)
	}
	waitForFresh(t, r)
	if got := r.Outputs(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("outputs = %q, want one, two", got)
	}

	// a failing command keeps its previous output
	if err := os.WriteFile(flag, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.Outputs()
	waitForFresh(t, r)
	if got := r.Outputs(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("outputs after failure = %q, want one, two", got)
	}
}

// A slow command must not block rendering.
func TestCommandRunnerDoesNotBlock(t *testing.T) {
	r := newCommandRunner("key 0", []string{"sleep 2; echo done"})

	start := time.Now()
	r.Outputs()
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("Outputs took %s", d)
	}
}

// failingWidget is a widget whose updates fail.
type failingWidget struct {
	*BaseWidget
	updates int
}

func (w *failingWidget) Update() error {
	w.updates++
	return errors.New("device unplugged")
}

// A failing widget must not shut deckmaster down, and isn't retried right
// away.
func TestUpdateWidgetsKeepsRunning(t *testing.T) {
	w := &failingWidget{BaseWidget: NewBaseWidget(xlDevice(), ".", 0, nil, nil, nil)}
	w.interval = time.Hour
	d := Deck{Widgets: []Widget{w}}

	d.updateWidgets()
	d.updateWidgets()

	if w.updates != 1 {
		t.Errorf("got %d updates, want 1", w.updates)
	}
	select {
	case err := <-shutdown:
		t.Fatalf("shutting down: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

// A top widget's mode is checked when the deck loads, not on every update.
func TestTopWidgetMode(t *testing.T) {
	for _, mode := range []string{"", "disk"} {
		path := writeDeck(t, `
[[keys]]
  index = 0
  [keys.widget]
    id = "top"
    [keys.widget.config]
      mode = "`+mode+`"
`)
		if _, err := LoadDeck(xlDevice(), ".", path); err == nil || !strings.Contains(err.Error(), "unknown mode") {
			t.Errorf("mode %q: got error %v, want unknown mode", mode, err)
		}
	}
}

func TestRecentWindowRange(t *testing.T) {
	path := writeDeck(t, `
[[keys]]
  index = 0
  [keys.widget]
    id = "recentWindow"
    [keys.widget.config]
      window = 256
`)
	if _, err := LoadDeck(xlDevice(), ".", path); err == nil || !strings.Contains(err.Error(), "window must be between 0 and 255") {
		t.Errorf("got error %v, want a window range error", err)
	}
}

func TestMsDuration(t *testing.T) {
	if got := msDuration(1500); got != 1500*time.Millisecond {
		t.Errorf("msDuration(1500) = %v", got)
	}
	if got := msDuration(^uint(0)); got <= 0 {
		t.Errorf("msDuration(max) = %v, want it to saturate", got)
	}
}
