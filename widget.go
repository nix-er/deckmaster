package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // decode PNG icons
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/nix-er/streamdeck"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	// DefaultColor is the standard color for text rendering.
	DefaultColor = color.RGBA{255, 255, 255, 255}
)

// Widget is an interface implemented by all available widgets.
type Widget interface {
	Key() uint8
	RequiresUpdate() bool
	Update() error
	Action() *ActionConfig
	ActionHold() *ActionConfig
	TriggerAction(hold bool)
	// Failed marks a failed update, so it's retried after the widget's
	// interval and not right away.
	Failed()
}

// output is what widgets show their images and LED colors on: the device,
// or a fake in tests.
type output interface {
	SetImage(index uint8, img image.Image) error
	SetScreenSegmentImage(index uint8, img image.Image) error
	SetInfoScreenImage(img image.Image) error
	SetKeyColor(index uint8, c color.Color) error
	SetDialKnobColor(index uint8, c color.Color) error
	SetDialRingColor(index uint8, c color.Color) error
	SetDialRingPercentage(index uint8, c color.Color, percent uint8) error
}

// BaseWidget provides common functionality required by all widgets.
type BaseWidget struct {
	base       string
	key        uint8
	action     *ActionConfig
	actionHold *ActionConfig
	dev        *streamdeck.Device
	out        output
	background image.Image
	lastUpdate time.Time
	interval   time.Duration

	// display is where the widget is shown.
	display display

	// led is the LED color of a touch key without a display (Stream Deck
	// Neo), nil to turn it off.
	led color.Color
}

// display is a display a widget can be shown on.
type display int

const (
	// displayKey is a key's display.
	displayKey display = iota
	// displaySegment is the touch screen segment above the dial with the
	// widget's index.
	displaySegment
	// displayInfo is the info screen (Stream Deck Neo).
	displayInfo
	// displayNone is used for keys and dials without a display (Stream Deck
	// Pedal and Studio, and the Neo's touch keys).
	displayNone
)

// canvas returns the area a widget can render on: the key, the whole touch
// screen segment or the info screen.
func (w *BaseWidget) canvas() image.Rectangle {
	switch w.display {
	case displaySegment:
		return image.Rect(0, 0, int(w.dev.ScreenSegmentWidth()), int(w.dev.ScreenHeight))
	case displayInfo:
		return image.Rect(0, 0, int(w.dev.InfoScreenWidth), int(w.dev.InfoScreenHeight))
	case displayNone:
		return image.Rectangle{}
	}
	return image.Rect(0, 0, int(w.dev.KeyWidth), int(w.dev.KeyHeight))
}

// pixels returns the size of the largest square image fitting the canvas.
func (w *BaseWidget) pixels() uint {
	c := w.canvas()
	return uint(min(c.Dx(), c.Dy())) //nolint:gosec // canvas sizes are positive
}

// dpi returns the DPI of the display a widget renders on.
func (w *BaseWidget) dpi() uint {
	if w.display == displaySegment {
		return w.dev.ScreenDPI
	}
	return w.dev.DPI
}

// Key returns the key a widget is mapped to.
func (w *BaseWidget) Key() uint8 {
	return w.key
}

// Action returns the associated ActionConfig.
func (w *BaseWidget) Action() *ActionConfig {
	return w.action
}

// ActionHold returns the associated ActionConfig for long presses.
func (w *BaseWidget) ActionHold() *ActionConfig {
	return w.actionHold
}

// TriggerAction gets called when a button is pressed.
func (w *BaseWidget) TriggerAction(_ bool) {
	// just a stub
}

// RequiresUpdate returns true when the widget wants to be repainted.
func (w *BaseWidget) RequiresUpdate() bool {
	if !w.lastUpdate.IsZero() && // initial paint done
		(w.interval == 0 || // never to be repainted
			time.Since(w.lastUpdate) < w.interval) {
		return false
	}

	return true
}

// Failed marks a failed update, so it's retried after the widget's interval.
func (w *BaseWidget) Failed() {
	w.lastUpdate = time.Now()
}

// Update renders the widget.
func (w *BaseWidget) Update() error {
	return w.render(nil)
}

// NewBaseWidget returns a new BaseWidget for a key.
func NewBaseWidget(dev *streamdeck.Device, base string, index uint8, action, actionHold *ActionConfig, bg image.Image) *BaseWidget {
	d := displayKey
	if index >= dev.Keys || dev.KeyWidth == 0 {
		d = displayNone
	}

	return &BaseWidget{
		base:       base,
		key:        index,
		action:     action,
		actionHold: actionHold,
		dev:        dev,
		out:        dev,
		background: bg,
		display:    d,
	}
}

// NewWidget initializes a widget.
func NewWidget(dev *streamdeck.Device, base string, kc KeyConfig, bg image.Image) (Widget, error) {
	bw := NewBaseWidget(dev, base, kc.Index, kc.Action, kc.ActionHold, bg)
	if kc.Color != "" {
		led, err := parseColor(kc.Color)
		if err != nil {
			return nil, fmt.Errorf("key %d: %w", kc.Index, err)
		}
		bw.led = led
	}

	if bw.display == displayNone {
		if kc.Widget.ID != "" {
			verbosef("Key %d has no display, ignoring its %s widget", kc.Index, kc.Widget.ID)
		}
		return bw, nil
	}

	widget, err := newWidget(bw, kc.Widget)
	if err != nil {
		return nil, fmt.Errorf("key %d: %w", kc.Index, err)
	}
	return widget, nil
}

// NewDialWidget initializes the widget shown on the touch screen segment
// above a dial. Dials don't need a widget, the segment then stays blank. On
// devices with LED rings around the dials, it returns a RingWidget instead.
func NewDialWidget(dev *streamdeck.Device, base string, dc DialConfig) (Widget, error) {
	bw := NewBaseWidget(dev, base, dc.Index, dc.Action, dc.ActionHold, nil)
	bw.display = displaySegment
	if dev.ScreenSegments == 0 {
		bw.display = displayNone
	}

	if dev.DialRings {
		return NewRingWidget(bw, dc)
	}
	if dc.Widget.ID == "" {
		return bw, nil
	}
	if bw.display == displayNone {
		verbosef("Dial %d has no display, ignoring its %s widget", dc.Index, dc.Widget.ID)
		return bw, nil
	}

	widget, err := newWidget(bw, dc.Widget)
	if err != nil {
		return nil, fmt.Errorf("dial %d: %w", dc.Index, err)
	}
	return widget, nil
}

// NewInfoWidget initializes the widget shown on the info screen (Stream Deck
// Neo).
func NewInfoWidget(dev *streamdeck.Device, base string, ic InfoConfig) (Widget, error) {
	bw := NewBaseWidget(dev, base, 0, nil, nil, nil)
	bw.display = displayInfo

	widget, err := newWidget(bw, ic.Widget)
	if err != nil {
		return nil, fmt.Errorf("info screen: %w", err)
	}
	return widget, nil
}

// parseColor parses a color in hex notation, like #ff8000.
func parseColor(s string) (color.Color, error) {
	c, err := colorful.Hex(s)
	if err != nil {
		return nil, fmt.Errorf("invalid color %q: %w", s, err)
	}
	return c, nil
}

func newWidget(bw *BaseWidget, wc WidgetConfig) (Widget, error) {
	var w Widget
	var err error
	switch wc.ID {
	case "button":
		w, err = NewButtonWidget(bw, wc)

	case "clock":
		w, err = NewTimeWidget(bw, withDefaults(wc, "%H;%i;%s", "bold;regular;thin"))

	case "date":
		w, err = NewTimeWidget(bw, withDefaults(wc, "%l;%d;%M", "regular;bold;regular"))

	case "time":
		w, err = NewTimeWidget(bw, wc)

	case "recentWindow":
		w, err = NewRecentWindowWidget(bw, wc)

	case "top":
		w, err = NewTopWidget(bw, wc)

	case "command":
		w, err = NewCommandWidget(bw, wc)

	case "weather":
		w, err = NewWeatherWidget(bw, wc)

	default:
		return nil, fmt.Errorf("unknown widget with ID %s", wc.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("%s widget: %w", wc.ID, err)
	}
	return w, nil
}

// withDefaults returns the config of a clock or date widget, using the given
// format and fonts unless they are configured. Other options, like colors,
// are kept.
func withDefaults(wc WidgetConfig, format, font string) WidgetConfig {
	config := maps.Clone(wc.Config)
	if config == nil {
		config = map[string]interface{}{}
	}
	if _, ok := config["format"]; !ok {
		config["format"] = format
	}
	if _, ok := config["font"]; !ok {
		config["font"] = font
	}
	wc.Config = config
	return wc
}

// renders the widget including its background image.
func (w *BaseWidget) render(fg image.Image) error {
	w.lastUpdate = time.Now()

	img := image.NewRGBA(w.canvas())
	if w.background != nil {
		draw.Draw(img, img.Bounds(), w.background, image.Point{}, draw.Over)
	}
	if fg != nil {
		// center images smaller than the canvas, e.g. square widget images
		// on a touch screen segment
		offset := image.Pt((img.Bounds().Dx()-fg.Bounds().Dx())/2, (img.Bounds().Dy()-fg.Bounds().Dy())/2)
		draw.Draw(img, fg.Bounds().Add(offset), fg, fg.Bounds().Min, draw.Over)
	}

	switch w.display {
	case displaySegment:
		return w.out.SetScreenSegmentImage(w.key, img)
	case displayInfo:
		return w.out.SetInfoScreenImage(img)
	case displayNone:
		if w.key >= w.dev.Keys && w.key < w.dev.Keys+w.dev.TouchKeys {
			return w.out.SetKeyColor(w.key, ledColor(w.led))
		}
		return nil
	}
	return w.out.SetImage(w.key, img)
}

// ledColor returns the color to set an LED to, black (off) for nil.
func ledColor(c color.Color) color.Color {
	if c == nil {
		return color.Black
	}
	return c
}

// change the interval a widget gets rendered in.
func (w *BaseWidget) setInterval(interval time.Duration, defaultInterval time.Duration) {
	if interval == 0 {
		interval = defaultInterval
	}

	w.interval = interval
}

// msDuration converts a millisecond count from a deck file to a duration,
// saturating instead of overflowing.
func msDuration(ms uint) time.Duration {
	if uint64(ms) > math.MaxInt64/uint64(time.Millisecond) {
		return math.MaxInt64
	}
	return time.Duration(ms) * time.Millisecond //nolint:gosec // bounded above
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return rasterizeSVG(f)
	}

	icon, _, err := image.Decode(f)
	return icon, err
}

func loadThemeImage(theme string, img string) (image.Image, error) {
	dir := filepath.Join("~", ".local", "share", "deckmaster", "themes", theme)
	abs, err := expandPath("", dir)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(abs, img+".svg")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(abs, img+".png")
	}
	return loadImage(path)
}

func flattenImage(img image.Image, clr color.Color) image.Image {
	bounds := img.Bounds()
	flatten := image.NewRGBA(bounds)
	draw.Draw(flatten, flatten.Bounds(), img, image.Point{}, draw.Src)
	alphaThreshold := uint32(20000)

	for x := 0; x < bounds.Dx(); x++ {
		for y := 0; y < bounds.Dy(); y++ {
			_, _, _, alpha := flatten.At(x, y).RGBA()
			if alpha > alphaThreshold {
				flatten.Set(x, y, clr)
			} else {
				flatten.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}

	return flatten
}

func drawImage(img *image.RGBA, icon image.Image, size int, pt image.Point) error {
	if pt.X < 0 {
		xcenter := float64(img.Bounds().Dx()/2.0) - (float64(size) / 2.0)
		pt = image.Pt(int(xcenter), pt.Y)
	}
	if pt.Y < 0 {
		ycenter := float64(img.Bounds().Dy()/2.0) - (float64(size) / 2.0)
		pt = image.Pt(pt.X, int(ycenter))
	}

	rect := image.Rect(pt.X, pt.Y, pt.X+size, pt.Y+size)
	draw.CatmullRom.Scale(img, rect, icon, icon.Bounds(), draw.Src, nil)

	return nil
}

func drawString(img *image.RGBA, bounds image.Rectangle, ttf *opentype.Font, text string, dpi uint, fontsize float64, color color.Color, pt image.Point) {
	if fontsize <= 0 {
		// pick biggest available height to fit the string
		fontsize, _ = maxPointSize(text, ttf, dpi, bounds.Dx(), bounds.Dy())
	}

	face, err := newFace(ttf, dpi, fontsize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't render string: %s\n", err)
		return
	}

	if pt.X < 0 {
		// center horizontally
		actwidth := font.MeasureString(face, text).Floor()
		xcenter := float64(bounds.Dx())/2.0 - (float64(actwidth) / 2.0)
		pt = image.Pt(bounds.Min.X+int(xcenter), pt.Y)
	}
	if pt.Y < 0 {
		// center vertically
		actheight := math.Round(fontsize * float64(dpi) / 72.0)
		ycenter := float64(bounds.Dy()/2.0) + (actheight / 2.6)
		pt = image.Pt(pt.X, bounds.Min.Y+int(ycenter))
	}

	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color),
		Face: face,
		Dot:  fixed.P(pt.X, pt.Y),
	}
	d.DrawString(text)
}
