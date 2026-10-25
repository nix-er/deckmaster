package main

import (
	"image"
	"reflect"
	"testing"
)

func TestDefaultLayout(t *testing.T) {
	// a 90 pixel canvas has a 5 pixel margin
	canvas := image.Rect(0, 0, 100, 90)
	tests := []struct {
		frames int
		want   []image.Rectangle
	}{
		{0, []image.Rectangle{image.Rect(0, 5, 100, 85)}},
		{1, []image.Rectangle{image.Rect(0, 5, 100, 85)}},
		{2, []image.Rectangle{image.Rect(0, 5, 100, 45), image.Rect(0, 45, 100, 85)}},
	}
	for _, tt := range tests {
		if got := NewLayout(canvas).DefaultLayout(tt.frames); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("DefaultLayout(%d) = %v, want %v", tt.frames, got, tt.want)
		}
	}
}

func TestFormatLayout(t *testing.T) {
	canvas := image.Rect(0, 0, 100, 90)
	got := NewLayout(canvas).FormatLayout([]string{"10x20+30x40", "bad"}, 3)
	want := []image.Rectangle{
		image.Rect(10, 20, 40, 60),
		// invalid and missing frames are the default ones
		image.Rect(0, 31, 100, 57),
		image.Rect(0, 57, 100, 83),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FormatLayout = %v, want %v", got, want)
	}
}

func TestFormatFrame(t *testing.T) {
	if got, err := formatFrame("0x5+96x40"); err != nil || got != image.Rect(0, 5, 96, 45) {
		t.Errorf("formatFrame = %v, %v", got, err)
	}
	for _, s := range []string{"", "10x20", "10x20+30", "10+30x40", "ax20+30x40", "10x20+30xb"} {
		if _, err := formatFrame(s); err == nil {
			t.Errorf("formatFrame(%q): expected an error", s)
		}
	}
}
