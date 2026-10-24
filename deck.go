package main

import (
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/godbus/dbus/v5"
	"github.com/nix-er/streamdeck"
)

// maxDialSteps limits how often a dial action gets repeated for a single
// (fast) turn.
const maxDialSteps = 10

// Deck is a set of widgets.
type Deck struct {
	File       string
	Background image.Image
	Widgets    []Widget

	// DialWidgets are shown on the touch screen segments above the dials.
	DialWidgets []Widget
	Dials       map[uint8]DialConfig
	Swipe       SwipeConfig

	// InfoWidget is shown on the info screen (Stream Deck Neo), nil if
	// there is none.
	InfoWidget Widget
}

// LoadDeck loads a deck configuration.
func LoadDeck(dev *streamdeck.Device, base string, deck string) (*Deck, error) {
	path, err := expandPath(base, deck)
	if err != nil {
		return nil, err
	}
	verbosef("Loading deck: %s", path)

	dc, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	d := Deck{
		File: path,
	}
	if dc.Background != "" {
		bgpath, err := expandPath(filepath.Dir(path), dc.Background)
		if err != nil {
			return nil, err
		}
		if err := d.loadBackground(dev, bgpath); err != nil {
			return nil, err
		}
	}

	keyMap := map[uint8]KeyConfig{}
	for _, k := range dc.Keys {
		keyMap[k.Index] = k
	}

	// touch keys follow the keys with displays
	for i := uint8(0); i < dev.Keys+dev.TouchKeys; i++ {
		bg := d.backgroundForKey(dev, i)

		var w Widget
		if k, found := keyMap[i]; found {
			w, err = NewWidget(dev, filepath.Dir(path), k, bg)
			if err != nil {
				return nil, err
			}
		} else {
			w = NewBaseWidget(dev, filepath.Dir(path), i, nil, nil, bg)
		}

		d.Widgets = append(d.Widgets, w)
	}

	d.Dials = map[uint8]DialConfig{}
	for _, c := range dc.Dials {
		d.Dials[c.Index] = c
	}
	d.Swipe = dc.Swipe

	for i := uint8(0); i < dev.Dials; i++ {
		c, found := d.Dials[i]
		if !found {
			c = DialConfig{Index: i}
		}

		w, err := NewDialWidget(dev, filepath.Dir(path), c)
		if err != nil {
			return nil, err
		}
		d.DialWidgets = append(d.DialWidgets, w)
	}

	if dev.InfoScreenWidth > 0 && dc.Info.Widget.ID != "" {
		d.InfoWidget, err = NewInfoWidget(dev, filepath.Dir(path), dc.Info)
		if err != nil {
			return nil, err
		}
	}

	return &d, nil
}

// loads a background image.
func (d *Deck) loadBackground(dev *streamdeck.Device, bg string) error {
	f, err := os.Open(bg)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	background, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	rows := int(dev.Rows)
	cols := int(dev.Columns)
	padding := int(dev.Padding)
	keyWidth := int(dev.KeyWidth)
	keyHeight := int(dev.KeyHeight)
	if keyWidth == 0 {
		verbosef("Device has no key displays, ignoring the background image")
		return nil
	}

	width := cols*keyWidth + (cols-1)*padding
	height := rows*keyHeight + (rows-1)*padding
	if background.Bounds().Dx() != width ||
		background.Bounds().Dy() != height {
		return fmt.Errorf("supplied background image has wrong dimensions, expected %dx%d pixels", width, height)
	}

	d.Background = background
	return nil
}

// returns the background image for an individual key, nil for keys without a
// display.
func (d Deck) backgroundForKey(dev *streamdeck.Device, key uint8) image.Image {
	if key >= dev.Keys || dev.KeyWidth == 0 {
		return nil
	}

	padding := int(dev.Padding)
	keyWidth := int(dev.KeyWidth)
	keyHeight := int(dev.KeyHeight)
	bg := image.NewRGBA(image.Rect(0, 0, keyWidth, keyHeight))

	if d.Background != nil {
		startx := int(key%dev.Columns) * (keyWidth + padding)
		starty := int(key/dev.Columns) * (keyHeight + padding)
		draw.Draw(bg, bg.Bounds(), d.Background, image.Point{startx, starty}, draw.Src)
	}

	return bg
}

// handles keypress with delay.
func emulateKeyPressWithDelay(keys string) {
	kd := strings.Split(keys, "+")
	emulateKeyPress(kd[0])
	if len(kd) == 1 {
		return
	}

	// optional delay
	if delay, err := strconv.Atoi(strings.TrimSpace(kd[1])); err == nil {
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

// emulates a range of key presses.
func emulateKeyPresses(keys string) {
	for _, kp := range strings.Split(keys, "/") {
		emulateKeyPressWithDelay(kp)
	}
}

// emulates a (multi-)key press.
func emulateKeyPress(keys string) {
	if keyboard == nil {
		fmt.Fprintln(os.Stderr, "Keyboard emulation is disabled!")
		return
	}

	kk := strings.Split(keys, "-")
	for i, k := range kk {
		k = formatKeycodes(strings.TrimSpace(k))
		kc, err := strconv.Atoi(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s is not a valid keycode: %s\n", k, err)
		}

		if i+1 < len(kk) {
			_ = keyboard.KeyDown(kc)
			defer keyboard.KeyUp(kc) //nolint:errcheck
		} else {
			_ = keyboard.KeyPress(kc)
		}
	}
}

// emulates a clipboard paste.
func emulateClipboard(text string) {
	err := clipboard.WriteAll(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Pasting to clipboard failed: %s\n", err)
	}

	// paste the string
	emulateKeyPress("29-47") // ctrl-v
}

// executes a dbus method.
func executeDBusMethod(object, path, method, args string) {
	call := dbusConn.Object(object, dbus.ObjectPath(path)).Call(method, 0, args)
	if call.Err != nil {
		fmt.Fprintf(os.Stderr, "dbus call failed: %s\n", call.Err)
	}
}

// executes a command.
func executeCommand(cmd string) {
	exp, err := expandPath("", cmd)
	if err == nil {
		cmd = exp
	}
	args := strings.Split(cmd, " ")

	c := exec.Command(args[0], args[1:]...) //nolint:gosec
	if *verbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}

	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Command failed: %s\n", err)
		return
	}
	if err := c.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Command failed: %s\n", err)
	}
}

// triggerAction triggers the action of a key.
func (d *Deck) triggerAction(dev *streamdeck.Device, index uint8, hold bool) {
	d.triggerWidgetAction(dev, d.Widgets, index, hold)
}

// triggerDialAction triggers the action of a pressed dial.
func (d *Deck) triggerDialAction(dev *streamdeck.Device, index uint8, hold bool) {
	d.triggerWidgetAction(dev, d.DialWidgets, index, hold)
	d.refreshRing(index)
}

// refreshRing updates the LED ring of a dial in the next update cycle, to show
// the effect of using the dial.
func (d *Deck) refreshRing(index uint8) {
	for _, w := range d.DialWidgets {
		if r, ok := w.(*RingWidget); ok && r.Key() == index {
			r.refresh()
		}
	}
}

// triggerWidgetAction triggers the (hold) action of the widget with the given
// index, or the widget's own action if none is configured.
func (d *Deck) triggerWidgetAction(dev *streamdeck.Device, widgets []Widget, index uint8, hold bool) {
	for _, w := range widgets {
		if w.Key() != index {
			continue
		}

		var a *ActionConfig
		if hold {
			a = w.ActionHold()
		} else {
			a = w.Action()
		}

		if a == nil {
			w.TriggerAction(hold)
			continue
		}

		d.runAction(dev, a)
	}
}

// turnDial triggers the left or right action of a dial, once per step.
func (d *Deck) turnDial(dev *streamdeck.Device, index uint8, steps int8) {
	defer d.refreshRing(index)

	dc, ok := d.Dials[index]
	if !ok {
		return
	}

	a := dc.ActionRight
	n := int(steps)
	if n < 0 {
		a = dc.ActionLeft
		n = -n
	}
	if a == nil {
		return
	}
	if n > maxDialSteps {
		n = maxDialSteps
	}
	if a.Deck != "" {
		// switching decks repeatedly makes no sense
		n = 1
	}

	for i := 0; i < n; i++ {
		d.runAction(dev, a)
	}
}

// touchSegment triggers the touch action of the dial below a touch screen
// segment, or the segment widget's own action if none is configured.
func (d *Deck) touchSegment(dev *streamdeck.Device, index uint8, hold bool) {
	dc := d.Dials[index]
	a := dc.ActionTouch
	if hold {
		a = dc.ActionTouchHold
	}
	if a != nil {
		d.runAction(dev, a)
		return
	}

	for _, w := range d.DialWidgets {
		if w.Key() == index {
			w.TriggerAction(hold)
		}
	}
}

// swipe triggers the action for a swipe to the left or right.
func (d *Deck) swipe(dev *streamdeck.Device, left bool) {
	a := d.Swipe.Right
	if left {
		a = d.Swipe.Left
	}
	if a != nil {
		d.runAction(dev, a)
	}
}

// runAction runs an action.
func (d *Deck) runAction(dev *streamdeck.Device, a *ActionConfig) {
	if a.Deck != "" {
		d, err := LoadDeck(dev, filepath.Dir(d.File), a.Deck)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Can't load deck:", err)
			return
		}
		if err := dev.Clear(); err != nil {
			fatal(err)
			return
		}

		deck = d
		deck.updateWidgets()
	}
	if a.Keycode != "" {
		emulateKeyPresses(a.Keycode)
	}
	if a.Paste != "" {
		emulateClipboard(a.Paste)
	}
	if a.DBus.Method != "" {
		executeDBusMethod(a.DBus.Object, a.DBus.Path, a.DBus.Method, a.DBus.Value)
	}
	if a.Exec != "" {
		go executeCommand(a.Exec)
	}
	if a.Device != "" {
		switch {
		case a.Device == "sleep":
			if err := dev.Sleep(); err != nil {
				fatalf("error: %v\n", err)
			}

		case strings.HasPrefix(a.Device, "brightness"):
			d.adjustBrightness(dev, strings.TrimPrefix(a.Device, "brightness"))

		default:
			fmt.Fprintln(os.Stderr, "Unrecognized special action:", a.Device)
		}
	}
}

// updateWidgets updates/repaints all the widgets.
func (d *Deck) updateWidgets() {
	groups := [][]Widget{d.Widgets, d.DialWidgets}
	if d.InfoWidget != nil {
		groups = append(groups, []Widget{d.InfoWidget})
	}
	for _, widgets := range groups {
		for _, w := range widgets {
			if !w.RequiresUpdate() {
				continue
			}

			// fmt.Println("Repaint", w.Key())
			if err := w.Update(); err != nil {
				fatalf("error: %v", err)
			}
		}
	}
}

// adjustBrightness adjusts the brightness.
func (d *Deck) adjustBrightness(dev *streamdeck.Device, value string) {
	if len(value) == 0 {
		fmt.Fprintln(os.Stderr, "No brightness value specified")
		return
	}

	v := int64(math.MinInt64)
	if len(value) > 1 {
		nv, err := strconv.ParseInt(value[1:], 10, 64)
		if err == nil {
			v = nv
		}
	}

	switch value[0] {
	case '=': // brightness=[n]:
	case '-': // brightness-[n]:
		if v == math.MinInt64 {
			v = 10
		}
		v = int64(*brightness) - v
	case '+': // brightness+[n]:
		if v == math.MinInt64 {
			v = 10
		}
		v = int64(*brightness) + v
	default:
		v = math.MinInt64
	}

	if v == math.MinInt64 {
		fmt.Fprintf(os.Stderr, "Could not grok the brightness from value '%s'\n", value)
		return
	}

	if v < 1 {
		v = 1
	} else if v > 100 {
		v = 100
	}
	if err := dev.SetBrightness(uint8(v)); err != nil {
		fatalf("error: %v\n", err)
	}

	*brightness = uint(v)
}
