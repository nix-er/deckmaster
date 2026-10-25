package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nix-er/streamdeck"
)

// withDeck makes d the current deck.
func withDeck(t *testing.T, d *Deck) {
	t.Helper()
	old := deck
	deck = d
	t.Cleanup(func() { deck = old })
}

func windowIDs() []uint32 {
	var ids []uint32
	for _, w := range recentWindows {
		ids = append(ids, w.ID)
	}
	return ids
}

func TestRecentWindows(t *testing.T) {
	withDeck(t, &Deck{})
	old := recentWindows
	t.Cleanup(func() { recentWindows = old })
	recentWindows = nil

	dev := pedalDevice() // 3 keys
	for _, id := range []uint32{1, 2, 3, 2, 4} {
		handleActiveWindowChanged(dev, ActiveWindowChangedEvent{Window: Window{ID: id}})
	}
	// the most recent window comes first, without duplicates, one per key
	if want := []uint32{4, 2, 3}; !reflect.DeepEqual(windowIDs(), want) {
		t.Errorf("recent windows = %v, want %v", windowIDs(), want)
	}

	handleWindowClosed(WindowClosedEvent{Window: Window{ID: 2}})
	if want := []uint32{4, 3}; !reflect.DeepEqual(windowIDs(), want) {
		t.Errorf("recent windows after closing 2 = %v, want %v", windowIDs(), want)
	}
}

// actionWidget records triggered actions.
type actionWidget struct {
	*BaseWidget
	triggered []bool
}

func (w *actionWidget) TriggerAction(hold bool) {
	w.triggered = append(w.triggered, hold)
}

func TestHandleTouch(t *testing.T) {
	dev := plusDevice() // 4 segments of 200 pixels
	tests := []struct {
		name  string
		touch streamdeck.Touch
		want  []string
	}{
		{"tap", streamdeck.Touch{Type: streamdeck.TouchTap, X: 250}, []string{"+2", "-2"}},
		{"long press", streamdeck.Touch{Type: streamdeck.TouchLongPress, X: 250}, []string{"+3", "-3"}},
		{"tap beyond the last segment", streamdeck.Touch{Type: streamdeck.TouchTap, X: 900}, []string{"+4", "-4"}},
		{"swipe left", streamdeck.Touch{Type: streamdeck.TouchSwipe, X: 600, EndX: 100}, []string{"+5", "-5"}},
		{"swipe right", streamdeck.Touch{Type: streamdeck.TouchSwipe, X: 100, EndX: 600}, []string{"+6", "-6"}},
		{"short swipe", streamdeck.Touch{Type: streamdeck.TouchSwipe, X: 100, EndX: 110}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withDeck(t, &Deck{
				Dials: map[uint8]DialConfig{
					1: {ActionTouch: &ActionConfig{Keycode: "2"}, ActionTouchHold: &ActionConfig{Keycode: "3"}},
					3: {ActionTouch: &ActionConfig{Keycode: "4"}},
				},
				Swipe: SwipeConfig{Left: &ActionConfig{Keycode: "5"}, Right: &ActionConfig{Keycode: "6"}},
			})
			kb := withFakeKeyboard(t)
			handleTouch(dev, tt.touch)
			if !reflect.DeepEqual(kb.events, tt.want) {
				t.Errorf("pressed %v, want %v", kb.events, tt.want)
			}
		})
	}
}

// Touching a segment without an action triggers its widget's own action.
func TestHandleTouchWidgetAction(t *testing.T) {
	dev := plusDevice()
	w := &actionWidget{BaseWidget: NewBaseWidget(dev, ".", 2, nil, nil, nil)}
	withDeck(t, &Deck{DialWidgets: []Widget{w}})

	handleTouch(dev, streamdeck.Touch{Type: streamdeck.TouchTap, X: 450})
	handleTouch(dev, streamdeck.Touch{Type: streamdeck.TouchLongPress, X: 450})
	if want := []bool{false, true}; !reflect.DeepEqual(w.triggered, want) {
		t.Errorf("triggered %v, want %v", w.triggered, want)
	}
}

// Devices without a touch screen ignore touches.
func TestHandleTouchWithoutScreen(t *testing.T) {
	withDeck(t, &Deck{Dials: map[uint8]DialConfig{0: {ActionTouch: &ActionConfig{Keycode: "2"}}}})
	kb := withFakeKeyboard(t)
	handleTouch(xlDevice(), streamdeck.Touch{Type: streamdeck.TouchTap})
	if len(kb.events) != 0 {
		t.Errorf("pressed %v, want nothing", kb.events)
	}
}

func TestTriggerAction(t *testing.T) {
	dev := xlDevice()
	plain := &actionWidget{BaseWidget: NewBaseWidget(dev, ".", 0, nil, nil, nil)}
	configured := &actionWidget{BaseWidget: NewBaseWidget(dev, ".", 1,
		&ActionConfig{Keycode: "2"}, &ActionConfig{Keycode: "3"}, nil)}
	d := &Deck{Widgets: []Widget{plain, configured}}

	kb := withFakeKeyboard(t)
	d.triggerAction(dev, 0, false)
	d.triggerAction(dev, 1, false)
	d.triggerAction(dev, 1, true)
	if want := []bool{false}; !reflect.DeepEqual(plain.triggered, want) {
		t.Errorf("widget without actions triggered %v, want %v", plain.triggered, want)
	}
	if len(configured.triggered) != 0 {
		t.Errorf("widget with actions triggered its own action")
	}
	if want := []string{"+2", "-2", "+3", "-3"}; !reflect.DeepEqual(kb.events, want) {
		t.Errorf("pressed %v, want %v", kb.events, want)
	}
}

func TestRunActionExec(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "ran")
	d := &Deck{}
	d.runAction(xlDevice(), &ActionConfig{Exec: "touch " + flag})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(flag); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command didn't run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A deck action switches to that deck, relative to the current one.
func TestRunActionDeck(t *testing.T) {
	dir := t.TempDir()
	next := filepath.Join(dir, "next.deck")
	if err := os.WriteFile(next, []byte("[[keys]]\n  index = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	current := &Deck{File: filepath.Join(dir, "main.deck")}
	withDeck(t, current)

	// the Pedal has no display that needs to be cleared
	current.runAction(pedalDevice(), &ActionConfig{Deck: "next.deck"})
	if deck == current || deck.File != next {
		t.Fatalf("current deck is %q, want %q", deck.File, next)
	}

	// a deck that can't be loaded keeps the current one
	loaded := deck
	loaded.runAction(pedalDevice(), &ActionConfig{Deck: "missing.deck"})
	if deck != loaded {
		t.Errorf("switched to %q after a failed load", deck.File)
	}
}
