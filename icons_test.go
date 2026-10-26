package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// testSVG is a red square covering the middle of a 24x24 viewBox, the shape of
// a Simple Icons icon with its fill on the root element.
const testSVG = `<svg fill="#FF0000" role="img" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">` +
	`<title>Test</title><path d="M6 6h12v12H6z"/></svg>`

// writeFile writes a file, creating its directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// iconEnv points the icon lookup at fresh directories and returns them.
func iconEnv(t *testing.T) (iconPath, dataHome, dataDir string) {
	t.Helper()
	base := t.TempDir()
	iconPath = filepath.Join(base, "path")
	dataHome = filepath.Join(base, "home")
	dataDir = filepath.Join(base, "dirs")
	t.Setenv("DECKMASTER_ICON_PATH", iconPath)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", dataDir)
	return iconPath, filepath.Join(dataHome, "deckmaster", "icons"), filepath.Join(dataDir, "deckmaster", "icons")
}

func TestLoadImageSVG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.svg")
	writeFile(t, path, testSVG)

	img, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if size := img.Bounds().Size(); size.X != svgSize || size.Y != svgSize {
		t.Errorf("image is %v, want %dx%d", size, svgSize, svgSize)
	}
	if got := color.RGBAModel.Convert(img.At(svgSize/2, svgSize/2)); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("center pixel = %v, want red", got)
	}
	if _, _, _, a := img.At(2, 2).RGBA(); a != 0 {
		t.Errorf("corner pixel alpha = %d, want transparent", a)
	}
}

// A non-square viewBox is centered, keeping its aspect ratio.
func TestLoadImageSVGAspect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide.svg")
	writeFile(t, path, `<svg fill="#fff" viewBox="0 0 20 10" xmlns="http://www.w3.org/2000/svg"><path d="M0 0h20v10H0z"/></svg>`)

	img, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(svgSize/2, svgSize/2).RGBA(); a == 0 {
		t.Error("center pixel is transparent")
	}
	if _, _, _, a := img.At(svgSize/2, 10).RGBA(); a != 0 {
		t.Error("top pixel is opaque, want letterboxing")
	}
}

// Elements oksvg doesn't support, like Inkscape's <metadata>, are skipped.
func TestLoadImageSVGMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.svg")
	writeFile(t, path, `<svg viewBox="0 0 10 10" xmlns="http://www.w3.org/2000/svg"><metadata><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/></metadata><path fill="#f00" d="M0 0h10v10H0z"/></svg>`)

	img, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(img.At(svgSize/2, svgSize/2)); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("center pixel = %v, want red", got)
	}
}

func TestLoadImageBadSVG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.svg")
	writeFile(t, path, "not an svg")
	if _, err := loadImage(path); err == nil {
		t.Error("expected an error")
	}
}

func TestIsIconName(t *testing.T) {
	for icon, want := range map[string]bool{
		"firefox":              true,
		"simple-icons/firefox": true,
		"firefox.svg":          false,
		"icons/firefox.png":    false,
		"/abs/firefox":         false,
		"~/firefox":            false,
		"./firefox":            false,
	} {
		if got := isIconName(icon); got != want {
			t.Errorf("isIconName(%q) = %v, want %v", icon, got, want)
		}
	}
}

func TestFindIcon(t *testing.T) {
	iconPath, dataHome, dataDir := iconEnv(t)
	writeFile(t, filepath.Join(dataHome, "loose.png"), "")
	writeFile(t, filepath.Join(dataHome, "mine", "ours.svg"), "")
	writeFile(t, filepath.Join(dataDir, "simple-icons", "icons", "firefox.svg"), "")
	writeFile(t, filepath.Join(dataDir, "simple-icons", "icons", "ours.svg"), "")
	writeFile(t, filepath.Join(dataDir, "other", "both.png"), "")
	writeFile(t, filepath.Join(dataDir, "other", "both.svg"), "")
	writeFile(t, filepath.Join(iconPath, "nix", "first.svg"), "")
	writeFile(t, filepath.Join(dataHome, "mine", "first.svg"), "")

	tests := []struct {
		name string
		want string
	}{
		{"loose", filepath.Join(dataHome, "loose.png")},
		{"ours", filepath.Join(dataHome, "mine", "ours.svg")},
		{"firefox", filepath.Join(dataDir, "simple-icons", "icons", "firefox.svg")},
		{"simple-icons/ours", filepath.Join(dataDir, "simple-icons", "icons", "ours.svg")},
		{"both", filepath.Join(dataDir, "other", "both.svg")},
		{"first", filepath.Join(iconPath, "nix", "first.svg")},
	}
	for _, tt := range tests {
		got, err := findIcon(tt.name)
		if err != nil {
			t.Errorf("findIcon(%q): %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("findIcon(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}

	for _, name := range []string{"missing", "other/firefox", "simple-icons/"} {
		if _, err := findIcon(name); err == nil {
			t.Errorf("findIcon(%q): expected an error", name)
		}
	}
}

// Packs in a root are searched in name order.
func TestFindIconPackOrder(t *testing.T) {
	_, dataHome, _ := iconEnv(t)
	writeFile(t, filepath.Join(dataHome, "b", "x.svg"), "")
	writeFile(t, filepath.Join(dataHome, "a", "icons", "x.svg"), "")

	got, err := findIcon("x")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dataHome, "a", "icons", "x.svg"); got != want {
		t.Errorf("findIcon = %q, want %q", got, want)
	}
}

func TestButtonWidgetIconName(t *testing.T) {
	_, dataHome, _ := iconEnv(t)
	writeFile(t, filepath.Join(dataHome, "simple-icons", "icons", "test.svg"), testSVG)

	w, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "button", Config: map[string]interface{}{
		"icon": "test",
	}})
	if w.(*ButtonWidget).icon == nil {
		t.Fatal("icon not loaded")
	}
	if got := color.RGBAModel.Convert(out.images["key 0"].At(48, 48)); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("center pixel = %v, want red", got)
	}
}

// An icon name that's in no pack leaves the button without an icon.
func TestButtonWidgetIconNameMissing(t *testing.T) {
	iconEnv(t)
	w, err := NewWidget(xlDevice(), "", KeyConfig{Widget: WidgetConfig{ID: "button", Config: map[string]interface{}{
		"icon": "missing",
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if icon := w.(*ButtonWidget).icon; icon != nil {
		t.Errorf("got icon %v, want none", icon)
	}
}

func TestNormalizePathData(t *testing.T) {
	for d, want := range map[string]string{
		"M6 6h12v12H6z":                 "M 6 6 h 12 v 12 H 6 z",
		"a11.85 11.85 0 00-8.617 3.696": "a 11.85 11.85 0 0 0 -8.617 3.696",
		"A1 1 0 1110 10":                "A 1 1 0 1 1 10 10",
		"a1 1 0 01.5.5 1 1 0 10-1-1":    "a 1 1 0 0 1 .5 .5 1 1 0 1 0 -1 -1",
		"M.5.5l1e-2-1E+1,3":             "M .5 .5 l 1e-2 -1E+1 3",
		"M1 1e":                         "M1 1e",
		"M1 2 L 3,4":                    "M 1 2 L 3 4",
	} {
		if got := normalizePathData(d); got != want {
			t.Errorf("normalizePathData(%q) = %q, want %q", d, got, want)
		}
	}
}

// Simple Icons write arc flags without separators, which oksvg can't parse
// on its own.
func TestLoadImageSVGCompactArcs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "circle.svg")
	writeFile(t, path, `<svg fill="#00FF00" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">`+
		`<path d="M12 2a10 10 0 110 20 10 10 0 010-20z"/></svg>`)

	img, err := loadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(img.At(svgSize/2, svgSize/2)); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("center pixel = %v, want green", got)
	}
}
