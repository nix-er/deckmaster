package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/nix-er/streamdeck"
	"golang.org/x/image/font/gofont/goregular"
)

// fakeOutput records what widgets show, instead of sending it to a device.
type fakeOutput struct {
	images   map[string]image.Image
	colors   map[string]color.Color
	percents map[uint8]uint8
}

func newFakeOutput() *fakeOutput {
	return &fakeOutput{
		images:   map[string]image.Image{},
		colors:   map[string]color.Color{},
		percents: map[uint8]uint8{},
	}
}

func (o *fakeOutput) SetImage(index uint8, img image.Image) error {
	o.images[fmt.Sprintf("key %d", index)] = img
	return nil
}

func (o *fakeOutput) SetScreenSegmentImage(index uint8, img image.Image) error {
	o.images[fmt.Sprintf("segment %d", index)] = img
	return nil
}

func (o *fakeOutput) SetInfoScreenImage(img image.Image) error {
	o.images["info"] = img
	return nil
}

func (o *fakeOutput) SetKeyColor(index uint8, c color.Color) error {
	o.colors[fmt.Sprintf("key %d", index)] = c
	return nil
}

func (o *fakeOutput) SetDialKnobColor(index uint8, c color.Color) error {
	o.colors[fmt.Sprintf("knob %d", index)] = c
	return nil
}

func (o *fakeOutput) SetDialRingColor(index uint8, c color.Color) error {
	return o.SetDialRingPercentage(index, c, 100)
}

func (o *fakeOutput) SetDialRingPercentage(index uint8, c color.Color, percent uint8) error {
	o.colors[fmt.Sprintf("ring %d", index)] = c
	o.percents[index] = percent
	return nil
}

// image returns the image shown on a display, failing if there is none.
func (o *fakeOutput) image(t *testing.T, display string) image.Image {
	t.Helper()
	img, ok := o.images[display]
	if !ok {
		t.Fatalf("nothing shown on %s, got %v", display, o.images)
	}
	return img
}

var fontsOnce sync.Once

// loadTestFonts loads the fonts widgets draw text with.
func loadTestFonts(t *testing.T) {
	t.Helper()
	var err error
	fontsOnce.Do(func() { err = loadFonts() })
	if err != nil {
		t.Fatal(err)
	}
}

// renderWidget creates a widget on the given display and updates it once.
func renderWidget(t *testing.T, dev *streamdeck.Device, d display, wc WidgetConfig) (Widget, *fakeOutput) {
	t.Helper()
	loadTestFonts(t)

	bw := NewBaseWidget(dev, ".", 0, nil, nil, nil)
	bw.display = d
	out := newFakeOutput()
	bw.out = out
	w, err := newWidget(bw, wc)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	return w, out
}

// litPixels counts the pixels of an image that aren't black or transparent.
func litPixels(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && r+g+b > 0 {
				n++
			}
		}
	}
	return n
}

// checkDrawn checks an image fits its display and isn't blank.
func checkDrawn(t *testing.T, img image.Image, size image.Point) {
	t.Helper()
	if img.Bounds().Size() != size {
		t.Errorf("image is %v, want %v", img.Bounds().Size(), size)
	}
	if litPixels(img) == 0 {
		t.Error("image is blank")
	}
}

// writeIcon writes a white square icon with a transparent border.
func writeIcon(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.Set(x, y, color.White)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestButtonWidgetUpdate(t *testing.T) {
	icon := filepath.Join(t.TempDir(), "icon.png")
	writeIcon(t, icon)

	tests := []struct {
		name    string
		dev     *streamdeck.Device
		display display
		config  map[string]interface{}
		shownOn string
		size    image.Point
	}{
		{"label", xlDevice(), displayKey, map[string]interface{}{"label": "Hi"}, "key 0", image.Pt(96, 96)},
		{"icon", xlDevice(), displayKey, map[string]interface{}{"icon": icon}, "key 0", image.Pt(96, 96)},
		{"icon and label", xlDevice(), displayKey, map[string]interface{}{"icon": icon, "label": "Hi"}, "key 0", image.Pt(96, 96)},
		{"segment label", plusDevice(), displaySegment, map[string]interface{}{"label": "Hi"}, "segment 0", image.Pt(200, 100)},
		{"segment icon beside label", plusDevice(), displaySegment, map[string]interface{}{"icon": icon, "label": "Hi"}, "segment 0", image.Pt(200, 100)},
		{"info screen", neoDevice(), displayInfo, map[string]interface{}{"label": "Hi"}, "info", image.Pt(248, 58)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, out := renderWidget(t, tt.dev, tt.display, WidgetConfig{ID: "button", Config: tt.config})
			checkDrawn(t, out.image(t, tt.shownOn), tt.size)
		})
	}
}

// A wide canvas puts the icon on the left of the label.
func TestButtonWidgetIconBesideLabel(t *testing.T) {
	icon := filepath.Join(t.TempDir(), "icon.png")
	writeIcon(t, icon)

	_, out := renderWidget(t, plusDevice(), displaySegment, WidgetConfig{ID: "button", Config: map[string]interface{}{
		"icon": icon, "label": "Hi", "color": "#ff0000",
	}})
	img := out.image(t, "segment 0")

	// the icon is white, the label red
	b := img.Bounds()
	left := image.Rect(0, 0, b.Dx()/3, b.Dy())
	white, red := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, _, a := img.At(x, y).RGBA()
			switch {
			case a == 0:
			case r > 0xf000 && g > 0xf000:
				if !image.Pt(x, y).In(left) {
					t.Fatalf("icon pixel at %d,%d, want it on the left", x, y)
				}
				white++
			case r > 0xf000 && g < 0x1000:
				red++
			}
		}
	}
	if white == 0 || red == 0 {
		t.Errorf("got %d icon and %d label pixels, want both", white, red)
	}
}

func TestRenderBackground(t *testing.T) {
	bg := image.NewUniform(color.RGBA{0, 0, 255, 255})
	bw := NewBaseWidget(xlDevice(), ".", 3, nil, nil, bg)
	out := newFakeOutput()
	bw.out = out

	// a transparent foreground keeps the background visible
	if err := bw.render(image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatal(err)
	}
	img := out.image(t, "key 3")
	if got := img.At(0, 0); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("corner = %v, want the blue background", got)
	}
	if bw.RequiresUpdate() {
		t.Error("widget wants an update right after rendering")
	}
}

func TestRenderCentersForeground(t *testing.T) {
	bw := NewBaseWidget(plusDevice(), ".", 1, nil, nil, nil)
	bw.display = displaySegment
	out := newFakeOutput()
	bw.out = out

	fg := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			fg.Set(x, y, color.White)
		}
	}
	if err := bw.render(fg); err != nil {
		t.Fatal(err)
	}
	img := out.image(t, "segment 1")
	// the 200x100 segment has a 50 pixel margin left and right
	for _, tt := range []struct {
		x   int
		lit bool
	}{{49, false}, {50, true}, {149, true}, {150, false}} {
		_, _, _, a := img.At(tt.x, 50).RGBA()
		if (a > 0) != tt.lit {
			t.Errorf("pixel %d,50 lit = %v, want %v", tt.x, a > 0, tt.lit)
		}
	}
}

// The touch keys of a Neo have no display, only an LED.
func TestRenderTouchKeyLED(t *testing.T) {
	dev := neoDevice()
	for _, tt := range []struct {
		color string
		want  color.Color
	}{
		{"#ff0000", colorful.Color{R: 1}},
		{"", color.Black},
	} {
		w, err := NewWidget(dev, ".", KeyConfig{Index: 8, Color: tt.color}, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := newFakeOutput()
		w.(*BaseWidget).out = out
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
		if got := out.colors["key 8"]; got != tt.want {
			t.Errorf("color %q: LED = %v, want %v", tt.color, got, tt.want)
		}
		if len(out.images) != 0 {
			t.Errorf("color %q: showed images %v on a key without display", tt.color, out.images)
		}
	}
}

func TestTimeWidgetUpdate(t *testing.T) {
	for _, id := range []string{"clock", "date"} {
		t.Run(id, func(t *testing.T) {
			_, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: id})
			checkDrawn(t, out.image(t, "key 0"), image.Pt(96, 96))
		})
	}
}

func TestFormatTime(t *testing.T) {
	tm := time.Date(2026, time.March, 4, 15, 6, 7, 0, time.UTC)
	tests := map[string]string{
		"%Y-%m-%d":    "2026-03-04",
		"%y":          "26",
		"%F %M":       "March Mar",
		"%l %D":       "Wednesday Wed",
		"%H:%i:%s":    "15:06:07",
		"%h%a":        "03PM",
		"%t":          "UTC",
		"no verbs":    "no verbs",
		"%H%i, %d.%m": "1506, 04.03",
	}
	for format, want := range tests {
		if got := formatTime(tm, format); got != want {
			t.Errorf("formatTime(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestCommandWidgetUpdate(t *testing.T) {
	w, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "command", Config: map[string]interface{}{
		"command": "echo 42",
	}})
	// the first update has no output yet
	if n := litPixels(out.image(t, "key 0")); n != 0 {
		t.Errorf("first update drew %d pixels, want none", n)
	}

	waitForFresh(t, w.(*CommandWidget).commands)
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	checkDrawn(t, out.image(t, "key 0"), image.Pt(96, 96))
}

func TestTopWidgetUpdate(t *testing.T) {
	for _, mode := range []string{"cpu", "memory"} {
		t.Run(mode, func(t *testing.T) {
			_, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "top", Config: map[string]interface{}{
				"mode": mode,
			}})
			checkDrawn(t, out.image(t, "key 0"), image.Pt(96, 96))
		})
	}
}

func TestRecentWindowWidgetUpdate(t *testing.T) {
	old := recentWindows
	t.Cleanup(func() { recentWindows = old })

	recentWindows = nil
	w, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "recentWindow", Config: map[string]interface{}{
		"window": int64(1), "showTitle": true,
	}})
	if n := litPixels(out.image(t, "key 0")); n != 0 {
		t.Errorf("drew %d pixels without a window, want none", n)
	}
	if w.RequiresUpdate() {
		t.Error("widget wants an update without a window")
	}

	icon := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(icon, icon.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	recentWindows = []Window{
		{ID: 1, Name: "first"},
		{ID: 2, Name: "a very long window title", Icon: icon},
	}
	if !w.RequiresUpdate() {
		t.Fatal("widget doesn't want an update for its new window")
	}
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	checkDrawn(t, out.image(t, "key 0"), image.Pt(96, 96))
	if got := w.(*RecentWindowWidget).label; got != "a very lon" {
		t.Errorf("label = %q, want the title cut to 10 characters", got)
	}
	if w.RequiresUpdate() {
		t.Error("widget wants an update for the window it shows")
	}
}

func TestFlattenImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	img.Set(0, 0, color.RGBA{10, 20, 30, 255})
	img.Set(1, 0, color.RGBA{0, 0, 0, 10}) // nearly transparent
	red := color.RGBA{255, 0, 0, 255}

	flat := flattenImage(img, red)
	if got := flat.At(0, 0); got != red {
		t.Errorf("opaque pixel = %v, want %v", got, red)
	}
	for x := 1; x < 3; x++ {
		if got := flat.At(x, 0); got != (color.RGBA{}) {
			t.Errorf("pixel %d = %v, want transparent", x, got)
		}
	}
}

// A flattened button icon takes the button's color.
func TestButtonWidgetFlatten(t *testing.T) {
	icon := filepath.Join(t.TempDir(), "icon.png")
	writeIcon(t, icon)

	w, _ := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "button", Config: map[string]interface{}{
		"icon": icon, "flatten": true, "color": "#00ff00",
	}})
	if got := w.(*ButtonWidget).icon.At(8, 8); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("icon pixel = %v, want green", got)
	}
}

func TestDrawImageCenters(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	icon := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(icon, icon.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if err := drawImage(img, icon, 10, image.Pt(-1, -1)); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		pt  image.Point
		lit bool
	}{{image.Pt(4, 10), false}, {image.Pt(5, 5), true}, {image.Pt(14, 14), true}, {image.Pt(15, 10), false}} {
		_, _, _, a := img.At(tt.pt.X, tt.pt.Y).RGBA()
		if (a > 0) != tt.lit {
			t.Errorf("pixel %v lit = %v, want %v", tt.pt, a > 0, tt.lit)
		}
	}
}

func TestMaxPointSize(t *testing.T) {
	loadTestFonts(t)

	short, w1 := maxPointSize("1", ttfFont, 166, 96, 40)
	long, w2 := maxPointSize("a much longer text", ttfFont, 166, 96, 40)
	if long >= short {
		t.Errorf("longer text got size %v, want it smaller than %v", long, short)
	}
	for _, w := range []int{w1, w2} {
		if w > 96 {
			t.Errorf("text is %d pixels wide, want it to fit 96", w)
		}
	}
}

func TestFontByName(t *testing.T) {
	loadTestFonts(t)

	for name, want := range map[string]interface{}{
		"thin": ttfThinFont, "regular": ttfFont, "bold": ttfBoldFont, "fancy": ttfFont,
	} {
		if got := fontByName(name); got != want {
			t.Errorf("fontByName(%q) isn't the expected font", name)
		}
	}
}

func TestLoadFontOrFallback(t *testing.T) {
	ttf, err := loadFontOrFallback("NoSuchFont-Regular.ttf", goregular.TTF)
	if err != nil || ttf == nil {
		t.Errorf("got %v, %v; want the fallback font", ttf, err)
	}
	if _, err := loadFontOrFallback("NoSuchFont-Regular.ttf", []byte("no font")); err == nil ||
		!strings.Contains(err.Error(), "fallback font") {
		t.Errorf("got error %v, want a fallback font error", err)
	}
}

func TestLoadThemeImage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeIcon(t, filepath.Join(home, ".local", "share", "deckmaster", "themes", "mine", "sun.png"))

	if img, err := loadThemeImage("mine", "sun"); err != nil || img.Bounds().Dx() != 16 {
		t.Errorf("got %v, %v; want the theme's icon", img, err)
	}
	if _, err := loadThemeImage("mine", "moon"); err == nil {
		t.Error("expected an error for a missing icon")
	}
}
