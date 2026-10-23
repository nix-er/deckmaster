package main

import (
	"fmt"
	"os"

	"github.com/flopp/go-findfont"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

var (
	ttfFont     *opentype.Font
	ttfThinFont *opentype.Font
	ttfBoldFont *opentype.Font
)

// maxPointSize returns the maximum point size we can use to fit text inside
// width and height, as well as the resulting text-width in pixels.
func maxPointSize(text string, ttf *opentype.Font, dpi uint, width, height int) (float64, int) {
	// never let the font size exceed the requested height
	fontsize := float64(height<<6) / float64(dpi) / (64.0 / 72.0)

	// offset initial loop iteration
	fontsize++

	// find the biggest matching font size for the requested width
	var actwidth int
	for actwidth = width + 1; actwidth > width && fontsize > 1; fontsize-- {
		face, err := newFace(ttf, dpi, fontsize)
		if err != nil {
			return 0, 0
		}

		actwidth = font.MeasureString(face, text).Round()
	}

	return fontsize, actwidth
}

func fontByName(font string) *opentype.Font {
	switch font {
	case "thin":
		return ttfThinFont
	case "regular":
		return ttfFont
	case "bold":
		return ttfBoldFont
	default:
		return ttfFont
	}
}

// newFace returns a font face of the given size.
func newFace(ttf *opentype.Font, dpi uint, fontsize float64) (font.Face, error) {
	return opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    fontsize,
		DPI:     float64(dpi),
		Hinting: font.HintingFull,
	})
}

func loadFont(name string) (*opentype.Font, error) {
	fontPath, err := findfont.Find(name)
	if err != nil {
		return nil, err
	}

	ttf, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, err
	}

	return opentype.Parse(ttf)
}

// loadFonts loads the default fonts.
func loadFonts() error {
	var err error
	ttfFont, err = loadFont("Roboto-Regular.ttf")
	if err != nil {
		return fmt.Errorf("error loading font: %w", err)
	}

	ttfThinFont, err = loadFont("Roboto-Thin.ttf")
	if err != nil {
		return fmt.Errorf("error loading font: %w", err)
	}

	ttfBoldFont, err = loadFont("Roboto-Bold.ttf")
	if err != nil {
		return fmt.Errorf("error loading font: %w", err)
	}

	return nil
}
