package main

import (
	"image"
	"image/color"
)

// ButtonWidget is a simple widget displaying an icon and/or label.
type ButtonWidget struct {
	*BaseWidget

	icon     image.Image
	label    string
	fontsize float64
	color    color.Color
	flatten  bool
}

// NewButtonWidget returns a new ButtonWidget.
func NewButtonWidget(bw *BaseWidget, opts WidgetConfig) (*ButtonWidget, error) {
	bw.setInterval(msDuration(opts.Interval), 0)

	var icon, label string
	var fontsize float64
	var color color.Color
	var flatten bool
	r := configReader{config: opts.Config}
	r.read("icon", &icon)
	r.read("label", &label)
	r.read("fontsize", &fontsize)
	r.read("color", &color)
	r.read("flatten", &flatten)
	if r.err != nil {
		return nil, r.err
	}

	if color == nil {
		color = DefaultColor
	}

	w := &ButtonWidget{
		BaseWidget: bw,
		label:      label,
		fontsize:   fontsize,
		color:      color,
		flatten:    flatten,
	}
	if icon != "" {
		if err := w.LoadImage(icon); err != nil {
			return nil, err
		}
	}

	return w, nil
}

// LoadImage loads an image from disk.
func (w *ButtonWidget) LoadImage(path string) error {
	path, err := expandPath(w.base, path)
	if err != nil {
		return err
	}
	icon, err := loadImage(path)
	if err != nil {
		return err
	}

	w.SetImage(icon)
	return nil
}

// SetImage updates the widget's icon.
func (w *ButtonWidget) SetImage(img image.Image) {
	w.icon = img
	if w.flatten {
		w.icon = flattenImage(w.icon, w.color)
	}
}

// Update renders the widget.
func (w *ButtonWidget) Update() error {
	canvas := w.canvas()
	margin := canvas.Dy() / 18
	height := canvas.Dy() - (margin * 2)
	img := image.NewRGBA(canvas)

	var err error
	switch {
	case w.label != "" && w.icon != nil && canvas.Dx() > canvas.Dy():
		err = w.drawIconBesideLabel(img, margin, height)
	case w.label != "":
		err = w.drawLabel(img, margin, height)
	case w.icon != nil:
		err = drawImage(img, w.icon, height, image.Pt(-1, -1))
	}
	if err != nil {
		return err
	}

	return w.render(img)
}

// drawIconBesideLabel draws the icon on the left and the label on the right,
// for wide canvases (touch screen segments).
func (w *ButtonWidget) drawIconBesideLabel(img *image.RGBA, margin, height int) error {
	iconsize := height * 3 / 4
	if err := drawImage(img, w.icon, iconsize, image.Pt(2*margin, -1)); err != nil {
		return err
	}

	bounds := image.Rect(iconsize+3*margin, margin, img.Bounds().Dx()-margin, img.Bounds().Dy()-margin)
	drawString(img, bounds, ttfFont, w.label, w.dpi(), w.fontsize, w.color, image.Pt(-1, -1))
	return nil
}

// drawLabel draws the label, below the icon if there is one.
func (w *ButtonWidget) drawLabel(img *image.RGBA, margin, height int) error {
	bounds := img.Bounds()
	if bounds.Dx() > bounds.Dy() {
		// keep labels on neighbouring touch screen segments apart
		bounds = bounds.Inset(margin * 2)
	}

	if w.icon != nil {
		iconsize := int((float64(height) / 3.0) * 2.0)
		if err := drawImage(img, w.icon, iconsize, image.Pt(-1, margin)); err != nil {
			return err
		}

		bounds.Min.Y += iconsize + margin
		bounds.Max.Y -= margin
	}

	drawString(img, bounds, ttfFont, w.label, w.dpi(), w.fontsize, w.color, image.Pt(-1, -1))
	return nil
}
