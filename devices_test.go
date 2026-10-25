package main

import (
	"image"
	"testing"

	"github.com/nix-er/streamdeck"
)

// The devices below describe the hardware without a connection to it, so
// tests render widgets on a fakeOutput instead.

func neoDevice() *streamdeck.Device {
	return &streamdeck.Device{
		Columns: 4, Rows: 2, Keys: 8, KeyWidth: 96, KeyHeight: 96, Pixels: 96, DPI: 166, Padding: 16,
		TouchKeys: 2, InfoScreenWidth: 248, InfoScreenHeight: 58,
	}
}

func studioDevice() *streamdeck.Device {
	return &streamdeck.Device{
		Columns: 16, Rows: 2, Keys: 32, KeyWidth: 144, KeyHeight: 112, Pixels: 112, DPI: 166, Padding: 16,
		Dials: 2, DialRings: true,
	}
}

func pedalDevice() *streamdeck.Device {
	return &streamdeck.Device{Columns: 3, Rows: 1, Keys: 3}
}

func TestLoadDeckNeo(t *testing.T) {
	d, err := LoadDeck(neoDevice(), ".", "decks/neo.deck")
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Widgets) != 10 {
		t.Fatalf("got %d key widgets, want 8 keys and 2 touch keys", len(d.Widgets))
	}
	touch, ok := d.Widgets[9].(*BaseWidget)
	if !ok {
		t.Fatalf("touch key widget is a %T, want *BaseWidget", d.Widgets[9])
	}
	if touch.display != displayNone || touch.led == nil {
		t.Errorf("touch key has display %d and LED %v", touch.display, touch.led)
	}
	if a := touch.Action(); a == nil || a.Keycode != "VolumeUp" {
		t.Errorf("touch key action = %+v, want keycode VolumeUp", a)
	}

	if d.InfoWidget == nil {
		t.Fatal("no info screen widget")
	}
	tw, ok := d.InfoWidget.(*TimeWidget)
	if !ok {
		t.Fatalf("info widget is a %T, want *TimeWidget", d.InfoWidget)
	}
	if c := tw.canvas(); c != image.Rect(0, 0, 248, 58) {
		t.Errorf("info widget canvas = %v, want 248x58", c)
	}
}

func TestLoadDeckStudio(t *testing.T) {
	d, err := LoadDeck(studioDevice(), ".", "decks/studio.deck")
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Widgets) != 32 {
		t.Fatalf("got %d key widgets, want 32", len(d.Widgets))
	}
	clock, ok := d.Widgets[0].(*TimeWidget)
	if !ok {
		t.Fatalf("key 0 widget is a %T, want *TimeWidget", d.Widgets[0])
	}
	if c := clock.canvas(); c != image.Rect(0, 0, 144, 112) {
		t.Errorf("key canvas = %v, want 144x112", c)
	}
	if p := clock.pixels(); p != 112 {
		t.Errorf("key pixels = %d, want 112", p)
	}

	if len(d.DialWidgets) != 2 {
		t.Fatalf("got %d dial widgets, want 2", len(d.DialWidgets))
	}
	ring, ok := d.DialWidgets[0].(*RingWidget)
	if !ok {
		t.Fatalf("dial widget is a %T, want *RingWidget", d.DialWidgets[0])
	}
	if ring.ringColor == nil || ring.knobColor == nil || ring.value == nil || ring.interval == 0 {
		t.Errorf("ring widget = %+v, want colors, value command and interval", ring)
	}
	if ring.display != displayNone {
		t.Errorf("ring widget has display %d, want none", ring.display)
	}

	// using the dial updates its ring right away
	ring.lastUpdate = ring.lastUpdate.AddDate(1, 0, 0)
	d.refreshRing(0)
	if !ring.RequiresUpdate() {
		t.Error("ring doesn't update after the dial was used")
	}
}

func TestLoadDeckPedal(t *testing.T) {
	// widgets of the main deck are ignored, as the pedals have no displays
	d, err := LoadDeck(pedalDevice(), ".", "decks/main.deck")
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Widgets) != 3 {
		t.Fatalf("got %d key widgets, want 3", len(d.Widgets))
	}
	for i, w := range d.Widgets {
		bw, ok := w.(*BaseWidget)
		if !ok || bw.display != displayNone {
			t.Errorf("pedal %d widget is a %T with a display", i, w)
		}
	}
}

func TestInvalidKeyColor(t *testing.T) {
	_, err := NewWidget(neoDevice(), ".", KeyConfig{Index: 8, Color: "orange"}, nil)
	if err == nil {
		t.Error("expected error for invalid color")
	}
}

func TestMergeDeckConfigInfo(t *testing.T) {
	parent := &DeckConfig{Info: InfoConfig{Widget: WidgetConfig{ID: "clock"}}}

	if got := MergeDeckConfig(&DeckConfig{}, parent); got.Info.Widget.ID != "clock" {
		t.Errorf("info widget = %q, want inherited clock", got.Info.Widget.ID)
	}

	base := &DeckConfig{Info: InfoConfig{Widget: WidgetConfig{ID: "date"}}}
	if got := MergeDeckConfig(base, parent); got.Info.Widget.ID != "date" {
		t.Errorf("info widget = %q, want date", got.Info.Widget.ID)
	}
}
