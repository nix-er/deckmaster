package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png" // decode PNG icons
	"math"
	"os"
	"path/filepath"
	"time"

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
}

// BaseWidget provides common functionality required by all widgets.
type BaseWidget struct {
	base       string
	key        uint8
	action     *ActionConfig
	actionHold *ActionConfig
	dev        *streamdeck.Device
	background image.Image
	lastUpdate time.Time
	interval   time.Duration

	// segment is true for widgets shown on a touch screen segment (above
	// the dial with the same index) instead of a key.
	segment bool
}

// canvas returns the area a widget can render on: the key, or the whole touch
// screen segment.
func (w *BaseWidget) canvas() image.Rectangle {
	if w.segment {
		return image.Rect(0, 0, int(w.dev.ScreenSegmentWidth()), int(w.dev.ScreenHeight))
	}
	return image.Rect(0, 0, int(w.dev.Pixels), int(w.dev.Pixels))
}

// pixels returns the size of a square image filling the height of the canvas.
func (w *BaseWidget) pixels() uint {
	if w.segment {
		return w.dev.ScreenHeight
	}
	return w.dev.Pixels
}

// dpi returns the DPI of the display a widget renders on.
func (w *BaseWidget) dpi() uint {
	if w.segment {
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

// Update renders the widget.
func (w *BaseWidget) Update() error {
	return w.render(w.dev, nil)
}

// NewBaseWidget returns a new BaseWidget.
func NewBaseWidget(dev *streamdeck.Device, base string, index uint8, action, actionHold *ActionConfig, bg image.Image) *BaseWidget {
	return &BaseWidget{
		base:       base,
		key:        index,
		action:     action,
		actionHold: actionHold,
		dev:        dev,
		background: bg,
	}
}

// NewWidget initializes a widget.
func NewWidget(dev *streamdeck.Device, base string, kc KeyConfig, bg image.Image) (Widget, error) {
	bw := NewBaseWidget(dev, base, kc.Index, kc.Action, kc.ActionHold, bg)
	return newWidget(bw, kc.Widget)
}

// NewDialWidget initializes the widget shown on the touch screen segment
// above a dial. Dials don't need a widget, the segment then stays blank.
func NewDialWidget(dev *streamdeck.Device, base string, dc DialConfig) (Widget, error) {
	bw := NewBaseWidget(dev, base, dc.Index, dc.Action, dc.ActionHold, nil)
	bw.segment = true
	if dc.Widget.ID == "" {
		return bw, nil
	}
	return newWidget(bw, dc.Widget)
}

func newWidget(bw *BaseWidget, wc WidgetConfig) (Widget, error) {
	switch wc.ID {
	case "button":
		return NewButtonWidget(bw, wc)

	case "clock":
		wc.Config = make(map[string]interface{})
		wc.Config["format"] = "%H;%i;%s"
		wc.Config["font"] = "bold;regular;thin"
		return NewTimeWidget(bw, wc), nil

	case "date":
		wc.Config = make(map[string]interface{})
		wc.Config["format"] = "%l;%d;%M"
		wc.Config["font"] = "regular;bold;regular"
		return NewTimeWidget(bw, wc), nil

	case "time":
		return NewTimeWidget(bw, wc), nil

	case "recentWindow":
		return NewRecentWindowWidget(bw, wc)

	case "top":
		return NewTopWidget(bw, wc), nil

	case "command":
		return NewCommandWidget(bw, wc), nil

	case "weather":
		return NewWeatherWidget(bw, wc)
	}

	// unknown widget ID
	return nil, fmt.Errorf("unknown widget with ID %s", wc.ID)
}

// renders the widget including its background image.
func (w *BaseWidget) render(dev *streamdeck.Device, fg image.Image) error {
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

	if w.segment {
		return dev.SetScreenSegmentImage(w.key, img)
	}
	return dev.SetImage(w.key, img)
}

// change the interval a widget gets rendered in.
func (w *BaseWidget) setInterval(interval time.Duration, defaultInterval time.Duration) {
	if interval == 0 {
		interval = defaultInterval
	}

	w.interval = interval
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	icon, _, err := image.Decode(f)
	return icon, err
}

func loadThemeImage(theme string, img string) (image.Image, error) {
	path := filepath.Join("~", ".local", "share", "deckmaster", "themes", theme, img+".png")
	abs, err := expandPath("", path)
	if err != nil {
		return nil, err
	}
	return loadImage(abs)
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
