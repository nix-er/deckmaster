package main

import (
	"fmt"
	"os"

	"github.com/flopp/go-findfont"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
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

// loadFontOrFallback loads the named font. If it can't be found, it falls
// back to the given embedded font, so a missing Roboto isn't fatal.
func loadFontOrFallback(name string, fallback []byte) (*opentype.Font, error) {
	ttf, err := loadFont(name)
	if err == nil {
		return ttf, nil
	}
	fmt.Fprintf(os.Stderr, "Can't load font %s, using a built-in fallback: %s\n", name, err)

	ttf, err = opentype.Parse(fallback)
	if err != nil {
		return nil, fmt.Errorf("error loading fallback font: %w", err)
	}
	return ttf, nil
}

// loadFonts loads the default fonts.
func loadFonts() error {
	var err error
	ttfFont, err = loadFontOrFallback("Roboto-Regular.ttf", goregular.TTF)
	if err != nil {
		return err
	}

	// the Go fonts have no thin variant
	ttfThinFont, err = loadFontOrFallback("Roboto-Thin.ttf", goregular.TTF)
	if err != nil {
		return err
	}

	ttfBoldFont, err = loadFontOrFallback("Roboto-Bold.ttf", gobold.TTF)
	if err != nil {
		return err
	}

	return nil
}
