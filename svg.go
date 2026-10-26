package main

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"regexp"
	"strings"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// svgSize is the edge length SVG icons are rasterized at, before they get
// scaled down to the canvas like any other icon.
const svgSize = 256

// pathData matches the d attribute of SVG path elements.
var pathData = regexp.MustCompile(`(\sd=)("[^"]*"|'[^']*')`)

// rasterizeSVG renders an SVG into a square image, keeping its aspect ratio.
func rasterizeSVG(r io.Reader) (img image.Image, err error) {
	// oksvg has panicked on malformed files before; fail the icon, not deckmaster
	defer func() {
		if p := recover(); p != nil {
			img, err = nil, fmt.Errorf("can't render svg: %v", p)
		}
	}()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = pathData.ReplaceAllFunc(data, func(m []byte) []byte {
		sub := pathData.FindSubmatch(m)
		quote := sub[2][0]
		d := normalizePathData(string(sub[2][1 : len(sub[2])-1]))
		return fmt.Appendf(nil, "%s%c%s%c", sub[1], quote, d, quote)
	})

	// Skip elements oksvg doesn't support, such as the <metadata> Inkscape
	// writes into most app icons, instead of rejecting the whole file.
	svg, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, err
	}

	w, h := svg.ViewBox.W, svg.ViewBox.H
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("svg has no usable viewBox")
	}
	scale := svgSize / max(w, h)
	svg.SetTarget((svgSize-w*scale)/2, (svgSize-h*scale)/2, w*scale, h*scale)

	rgba := image.NewRGBA(image.Rect(0, 0, svgSize, svgSize))
	scanner := rasterx.NewScannerGV(svgSize, svgSize, rgba, rgba.Bounds())
	svg.Draw(rasterx.NewDasher(svgSize, svgSize, scanner), 1)
	return rgba, nil
}

// normalizePathData separates all commands and numbers of SVG path data by
// spaces. oksvg can't parse the compact forms the SVG grammar allows, such as
// arc flags without separators ("a1 1 0 00-2 2") or ".5.5", which minified
// icons, e.g. all of Simple Icons, are full of. Data it doesn't understand is
// returned unchanged.
func normalizePathData(d string) string {
	var b strings.Builder
	var cmd byte
	args := 0

	for i := 0; i < len(d); {
		c := d[i]
		switch {
		case c == ' ' || c == ',' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') && c != 'e' && c != 'E':
			cmd, args = c, 0
			b.WriteByte(' ')
			b.WriteByte(c)
			i++
		case (cmd == 'a' || cmd == 'A') && (args%7 == 3 || args%7 == 4) && (c == '0' || c == '1'):
			// arc flags are a single digit, and may be followed by anything
			b.WriteByte(' ')
			b.WriteByte(c)
			args++
			i++
		default:
			j := scanNumber(d, i)
			if j == i {
				return d
			}
			b.WriteByte(' ')
			b.WriteString(d[i:j])
			args++
			i = j
		}
	}

	return strings.TrimSpace(b.String())
}

// scanNumber returns the end of the number starting at d[i], or i if there
// is none.
func scanNumber(d string, i int) int {
	j := i
	if j < len(d) && (d[j] == '+' || d[j] == '-') {
		j++
	}
	digits := 0
	for ; j < len(d) && d[j] >= '0' && d[j] <= '9'; j++ {
		digits++
	}
	if j < len(d) && d[j] == '.' {
		j++
		for ; j < len(d) && d[j] >= '0' && d[j] <= '9'; j++ {
			digits++
		}
	}
	if digits == 0 {
		return i
	}

	// an exponent only counts if it has digits
	if j < len(d) && (d[j] == 'e' || d[j] == 'E') {
		k := j + 1
		if k < len(d) && (d[k] == '+' || d[k] == '-') {
			k++
		}
		if k < len(d) && d[k] >= '0' && d[k] <= '9' {
			for k < len(d) && d[k] >= '0' && d[k] <= '9' {
				k++
			}
			j = k
		}
	}
	return j
}
