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
	value     *commandRunner
	badValue  string
}

// NewRingWidget returns a new RingWidget.
func NewRingWidget(bw *BaseWidget, dc DialConfig) (*RingWidget, error) {
	w := &RingWidget{
		BaseWidget: bw,
	}
	if dc.RingValue != "" {
		w.value = newCommandRunner(fmt.Sprintf("dial %d", dc.Index), []string{dc.RingValue})
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

	if w.value != nil {
		bw.setInterval(time.Duration(dc.RingInterval)*time.Millisecond, time.Second)
	}
	return w, nil
}

// RequiresUpdate returns true when the LEDs need to be set.
func (w *RingWidget) RequiresUpdate() bool {
	return (w.value != nil && w.value.Fresh()) || w.BaseWidget.RequiresUpdate()
}

// Update sets the LEDs of the dial.
func (w *RingWidget) Update() error {
	w.lastUpdate = time.Now()

	if err := w.dev.SetDialKnobColor(w.key, ledColor(w.knobColor)); err != nil {
		return err
	}
	if w.value == nil {
		return w.dev.SetDialRingColor(w.key, ledColor(w.ringColor))
	}

	c := w.ringColor
	if c == nil {
		c = DefaultColor
	}
	return w.dev.SetDialRingPercentage(w.key, c, w.percent())
}

// percent returns the latest output of the ring value command as a
// percentage, 0 until it succeeds or if it's no number. The command runs in
// the background.
func (w *RingWidget) percent() uint8 {
	out := strings.TrimSpace(w.value.Outputs()[0])
	if out == "" {
		return 0
	}

	v, err := strconv.ParseFloat(strings.TrimSuffix(out, "%"), 64)
	if err != nil {
		if out != w.badValue {
			fmt.Fprintf(os.Stderr, "Ring value of dial %d is no number: %q\n", w.key, out)
			w.badValue = out
		}
		return 0
	}
	return uint8(min(max(v, 0), 100))
}

// refresh makes the widget update in the next update cycle, e.g. after the
// dial was used.
func (w *RingWidget) refresh() {
	w.lastUpdate = time.Time{}
}
