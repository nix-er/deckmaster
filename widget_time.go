package main

import (
	"image"
	"image/color"
	"strings"
	"time"
)

// TimeWidget is a widget displaying the current time/date.
type TimeWidget struct {
	*BaseWidget

	formats []string
	fonts   []string
	colors  []color.Color
	frames  []image.Rectangle
}

// NewTimeWidget returns a new TimeWidget.
func NewTimeWidget(bw *BaseWidget, opts WidgetConfig) (*TimeWidget, error) {
	bw.setInterval(msDuration(opts.Interval), time.Second/2)

	var formats, fonts, frameReps []string
	var colors []color.Color
	r := configReader{config: opts.Config}
	r.read("format", &formats)
	r.read("font", &fonts)
	r.read("layout", &frameReps)
	r.read("color", &colors)
	if r.err != nil {
		return nil, r.err
	}

	layout := NewLayout(bw.canvas())
	frames := layout.FormatLayout(frameReps, len(formats))

	for i := 0; i < len(formats); i++ {
		if len(fonts) < i+1 {
			fonts = append(fonts, "regular")
		}
		if len(colors) < i+1 {
			colors = append(colors, DefaultColor)
		}
	}

	return &TimeWidget{
		BaseWidget: bw,
		formats:    formats,
		fonts:      fonts,
		colors:     colors,
		frames:     frames,
	}, nil
}

// Update renders the widget.
func (w *TimeWidget) Update() error {
	img := image.NewRGBA(w.canvas())

	for i := 0; i < len(w.formats); i++ {
		str := formatTime(time.Now(), w.formats[i])
		font := fontByName(w.fonts[i])

		drawString(img,
			w.frames[i],
			font,
			str,
			w.dpi(),
			-1,
			w.colors[i],
			image.Pt(-1, -1))
	}

	return w.render(img)
}

func formatTime(t time.Time, format string) string {
	tm := map[string]string{
		"%Y": "2006",
		"%y": "06",
		"%F": "January",
		"%M": "Jan",
		"%m": "01",
		"%l": "Monday",
		"%D": "Mon",
		"%d": "02",
		"%h": "03",
		"%H": "15",
		"%i": "04",
		"%s": "05",
		"%a": "PM",
		"%t": "MST",
	}

	for k, v := range tm {
		format = strings.ReplaceAll(format, k, v)
	}

	return t.Format(format)
}
