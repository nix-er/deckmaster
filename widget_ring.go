package main

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"
)

// RingWidget shows a dial's state on its LED ring and knob LED (Stream Deck
// Studio).
type RingWidget struct {
	*BaseWidget

	ringColor color.Color
	knobColor color.Color
	value     string
}

// NewRingWidget returns a new RingWidget.
func NewRingWidget(bw *BaseWidget, dc DialConfig) (*RingWidget, error) {
	w := &RingWidget{
		BaseWidget: bw,
		value:      dc.RingValue,
	}

	var err error
	if dc.RingColor != "" {
		if w.ringColor, err = parseColor(dc.RingColor); err != nil {
			return nil, fmt.Errorf("dial %d: %w", dc.Index, err)
		}
	}
	if dc.KnobColor != "" {
		if w.knobColor, err = parseColor(dc.KnobColor); err != nil {
			return nil, fmt.Errorf("dial %d: %w", dc.Index, err)
		}
	}
	if dc.Widget.ID != "" {
		verbosef("Dial %d has no display, ignoring its %s widget", dc.Index, dc.Widget.ID)
	}

	if w.value != "" {
		bw.setInterval(time.Duration(dc.RingInterval)*time.Millisecond, time.Second)
	}
	return w, nil
}

// Update sets the LEDs of the dial.
func (w *RingWidget) Update() error {
	w.lastUpdate = time.Now()

	if err := w.dev.SetDialKnobColor(w.key, ledColor(w.knobColor)); err != nil {
		return err
	}
	if w.value == "" {
		return w.dev.SetDialRingColor(w.key, ledColor(w.ringColor))
	}

	c := w.ringColor
	if c == nil {
		c = DefaultColor
	}
	return w.dev.SetDialRingPercentage(w.key, c, w.percent())
}

// percent runs the ring value command and returns its output as a
// percentage, 0 if it fails.
func (w *RingWidget) percent() uint8 {
	out, err := runCommand(w.value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ring value command of dial %d failed: %s\n", w.key, err)
		return 0
	}

	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(out), "%"), 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ring value of dial %d is no number: %q\n", w.key, out)
		return 0
	}
	return uint8(min(max(v, 0), 100))
}

// refresh makes the widget update in the next update cycle, e.g. after the
// dial was used.
func (w *RingWidget) refresh() {
	w.lastUpdate = time.Time{}
}
