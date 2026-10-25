package main

import (
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestWeatherDataParse(t *testing.T) {
	tests := []struct {
		response    string
		condition   string
		temperature string
		err         bool
	}{
		{"o +12°C", "o", "12°C", false},
		{"mm -3°F", "mm", "-3°F", false},
		{"Unknown location; please try ~52.5,13.4", "", "", false},
		{"garbage", "", "", true},
		{"", "", "", true},
	}
	for _, tt := range tests {
		w := WeatherData{location: "nowhere", response: tt.response}
		cond, err := w.Condition()
		if cond != tt.condition || (err != nil) != tt.err {
			t.Errorf("Condition() of %q = %q, %v; want %q", tt.response, cond, err, tt.condition)
		}
		temp, err := w.Temperature()
		if temp != tt.temperature || (err != nil) != tt.err {
			t.Errorf("Temperature() of %q = %q, %v; want %q", tt.response, temp, err, tt.temperature)
		}
	}
}

func TestFormatUnit(t *testing.T) {
	for unit, want := range map[string]string{
		"f": "&u", "fahrenheit": "&u", "c": "&m", "celsius": "&m", "": "", "kelvin": "",
	} {
		if got := formatUnit(unit); got != want {
			t.Errorf("formatUnit(%q) = %q, want %q", unit, got, want)
		}
	}
}

// withWeatherServer sends weather requests to a test server.
func withWeatherServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	old := weatherClient
	weatherClient = &http.Client{Transport: rewriteTransport{target}}
	t.Cleanup(func() { weatherClient = old })
}

// rewriteTransport sends all requests to a test server.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestWeatherDataFetch(t *testing.T) {
	var requests []string
	withWeatherServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		_, _ = w.Write([]byte("o +20°C"))
	})

	d := WeatherData{location: "New York", unit: "c"}
	d.Fetch()
	if !d.Fresh() {
		t.Fatal("no fresh data after fetching")
	}
	if cond, _ := d.Condition(); cond != "o" {
		t.Errorf("condition = %q, want o", cond)
	}
	if want := "/New%20York?format=%x+%t&m"; len(requests) != 1 || requests[0] != want {
		t.Errorf("requests = %q, want %q", requests, want)
	}

	// the data is only fetched every 15 minutes
	d.Reset()
	d.Fetch()
	if d.Fresh() || len(requests) != 1 {
		t.Errorf("fetched again right away: %d requests", len(requests))
	}
}

func TestWeatherDataFetchFails(t *testing.T) {
	// nothing listens on port 1
	old := weatherClient
	weatherClient = &http.Client{Transport: rewriteTransport{&url.URL{Scheme: "http", Host: "127.0.0.1:1"}}}
	t.Cleanup(func() { weatherClient = old })

	d := WeatherData{location: "x"}
	d.Fetch()
	if d.Fresh() {
		t.Error("fresh data after a failed fetch")
	}
}

// Every condition wttr.in reports shows an icon.
func TestWeatherWidgetUpdate(t *testing.T) {
	for _, cond := range []string{
		"mm", "mmm", "m", "=", "///", "//", "x", "x/", "/", ".",
		"**", "*/*", "*", "*/", "/!/", "!/", "*!*", "o",
	} {
		t.Run(cond, func(t *testing.T) {
			w, out := newWeatherWidget(t, cond+" +5°C")
			if err := w.Update(); err != nil {
				t.Fatal(err)
			}
			if w.icon == nil {
				t.Fatal("no icon")
			}
			if w.label != "5°C" {
				t.Errorf("label = %q, want 5°C", w.label)
			}
			if w.data.Fresh() {
				t.Error("data still fresh after showing it")
			}
			checkDrawn(t, out.image(t, "key 0"), image.Pt(96, 96))
		})
	}
}

func TestWeatherWidgetUnknownCondition(t *testing.T) {
	for _, response := range []string{"?? +5°C", "garbage"} {
		w, out := newWeatherWidget(t, response)
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
		if w.icon != nil {
			t.Errorf("%q: got an icon", response)
		}
		if n := litPixels(out.image(t, "key 0")); n != 0 {
			t.Errorf("%q: drew %d pixels, want none", response, n)
		}
	}
}

// A theme's icons replace the built-in ones, which are the fallback for
// missing icons.
func TestWeatherWidgetTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeIcon(t, filepath.Join(home, ".local", "share", "deckmaster", "themes", "mine", "fog.png"))

	for cond, size := range map[string]int{"=": 16, "mm": weatherImage("assets/weather/cloudy.png").Bounds().Dx()} {
		w, _ := newWeatherWidget(t, cond+" +5°C")
		w.theme = "mine"
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
		if got := w.icon.Bounds().Dx(); got != size {
			t.Errorf("%q: icon is %d pixels wide, want %d", cond, got, size)
		}
	}
}

// newWeatherWidget returns a weather widget showing the given response,
// without fetching weather data.
func newWeatherWidget(t *testing.T, response string) (*WeatherWidget, *fakeOutput) {
	t.Helper()
	loadTestFonts(t)

	bw := NewBaseWidget(xlDevice(), ".", 0, nil, nil, nil)
	out := newFakeOutput()
	bw.out = out
	w, err := NewWeatherWidget(bw, WidgetConfig{ID: "weather"})
	if err != nil {
		t.Fatal(err)
	}
	w.data.refresh = time.Now()
	w.data.response = response
	w.data.fresh = true
	return w, out
}

// A weather widget repaints when new weather data arrives.
func TestWeatherWidgetRequiresUpdate(t *testing.T) {
	w, _ := newWeatherWidget(t, "o +5°C")
	w.lastUpdate = time.Now()
	if !w.RequiresUpdate() {
		t.Error("no update for fresh data")
	}
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	if w.RequiresUpdate() {
		t.Error("update without fresh data, before the interval is over")
	}
}
