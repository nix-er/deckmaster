package main

import (
	"image/color"
	"testing"

	colorful "github.com/lucasb-eyer/go-colorful"
)

// newRingWidget returns the ring widget of a Stream Deck Studio dial.
func newRingWidget(t *testing.T, dc DialConfig) (*RingWidget, *fakeOutput) {
	t.Helper()
	w, err := NewDialWidget(studioDevice(), ".", dc)
	if err != nil {
		t.Fatal(err)
	}
	ring, ok := w.(*RingWidget)
	if !ok {
		t.Fatalf("got a %T, want a RingWidget", w)
	}
	out := newFakeOutput()
	ring.out = out
	return ring, out
}

func TestRingWidgetColors(t *testing.T) {
	w, out := newRingWidget(t, DialConfig{Index: 1, RingColor: "#ff0000", KnobColor: "#0000ff"})
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	if got := out.colors["ring 1"]; got != (colorful.Color{R: 1}) {
		t.Errorf("ring = %v, want red", got)
	}
	if got := out.colors["knob 1"]; got != (colorful.Color{B: 1}) {
		t.Errorf("knob = %v, want blue", got)
	}
	if got := out.percents[1]; got != 100 {
		t.Errorf("ring is %d%% lit, want all of it", got)
	}
}

// Dials without colors have their LEDs turned off.
func TestRingWidgetOff(t *testing.T) {
	w, out := newRingWidget(t, DialConfig{Index: 0})
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	for _, led := range []string{"ring 0", "knob 0"} {
		if got := out.colors[led]; got != color.Black {
			t.Errorf("%s = %v, want off", led, got)
		}
	}
}

func TestRingWidgetInvalidColor(t *testing.T) {
	for _, dc := range []DialConfig{{RingColor: "red"}, {KnobColor: "#12"}} {
		if _, err := NewDialWidget(studioDevice(), ".", dc); err == nil {
			t.Errorf("%+v: expected an error", dc)
		}
	}
}

func TestRingWidgetValue(t *testing.T) {
	tests := []struct {
		output string
		want   uint8
	}{
		{"42", 42},
		{"42.9%", 42},
		{"150", 100},
		{"-5", 0},
		{"loud", 0},
		{"", 0},
	}
	for _, tt := range tests {
		w, out := newRingWidget(t, DialConfig{RingValue: "echo '" + tt.output + "'"})
		if !w.RequiresUpdate() {
			t.Fatal("no initial update")
		}
		// the command runs in the background, the first update shows 0
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
		waitForFresh(t, w.value)
		if !w.RequiresUpdate() {
			t.Errorf("%q: no update for the command's output", tt.output)
		}
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
		if got := out.percents[0]; got != tt.want {
			t.Errorf("%q: ring is %d%% lit, want %d%%", tt.output, got, tt.want)
		}
		if got := out.colors["ring 0"]; got != DefaultColor {
			t.Errorf("%q: ring = %v, want the default color", tt.output, got)
		}
	}
}
