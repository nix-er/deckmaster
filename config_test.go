package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func dialsByIndex(dials Dials) map[uint8]DialConfig {
	m := map[uint8]DialConfig{}
	for _, d := range dials {
		m[d.Index] = d
	}
	return m
}

func TestLoadConfigDials(t *testing.T) {
	dc, err := LoadConfig("decks/plus.deck")
	if err != nil {
		t.Fatal(err)
	}

	if len(dc.Keys) != 8 {
		t.Errorf("got %d keys, want 8", len(dc.Keys))
	}
	dials := dialsByIndex(dc.Dials)
	if len(dials) != 4 {
		t.Fatalf("got %d dials, want 4", len(dials))
	}

	volume := dials[0]
	if volume.Widget.ID != "button" {
		t.Errorf("dial 0 widget = %q, want button", volume.Widget.ID)
	}
	if volume.Action == nil || volume.Action.Keycode != "Mute" {
		t.Errorf("dial 0 action = %+v, want keycode Mute", volume.Action)
	}
	if volume.ActionLeft == nil || volume.ActionLeft.Keycode != "VolumeDown" {
		t.Errorf("dial 0 action_left = %+v, want keycode VolumeDown", volume.ActionLeft)
	}
	if volume.ActionRight == nil || volume.ActionRight.Keycode != "VolumeUp" {
		t.Errorf("dial 0 action_right = %+v, want keycode VolumeUp", volume.ActionRight)
	}
	if clock := dials[3]; clock.ActionTouch == nil || clock.ActionTouch.Device != "sleep" {
		t.Errorf("dial 3 action_touch = %+v, want device sleep", clock.ActionTouch)
	}

	if dc.Swipe.Left == nil || dc.Swipe.Left.Deck != "plus-media.deck" {
		t.Errorf("swipe.left = %+v, want deck plus-media.deck", dc.Swipe.Left)
	}
	if dc.Swipe.Right != nil {
		t.Errorf("swipe.right = %+v, want none", dc.Swipe.Right)
	}
}

func TestLoadConfigDialsFromParent(t *testing.T) {
	dc, err := LoadConfig("decks/plus-media.deck")
	if err != nil {
		t.Fatal(err)
	}

	if len(dc.Keys) != 8 {
		t.Errorf("got %d keys, want 8 inherited keys", len(dc.Keys))
	}
	dials := dialsByIndex(dc.Dials)
	if len(dials) != 4 {
		t.Fatalf("got %d dials, want 4", len(dials))
	}
	if dials[0].ActionRight == nil || dials[0].ActionRight.Keycode != "VolumeUp" {
		t.Errorf("dial 0 not inherited from parent: %+v", dials[0])
	}
	if dials[3].Widget.ID != "top" {
		t.Errorf("dial 3 widget = %q, want top (overridden)", dials[3].Widget.ID)
	}

	if dc.Swipe.Left == nil || dc.Swipe.Left.Deck != "plus.deck" {
		t.Errorf("swipe.left = %+v, want deck plus.deck", dc.Swipe.Left)
	}
	if dc.Swipe.Right == nil || dc.Swipe.Right.Deck != "plus.deck" {
		t.Errorf("swipe.right = %+v, want deck plus.deck", dc.Swipe.Right)
	}
}

func TestMergeDeckConfigSwipe(t *testing.T) {
	parent := DeckConfig{Swipe: SwipeConfig{
		Left:  &ActionConfig{Deck: "parent-left.deck"},
		Right: &ActionConfig{Deck: "parent-right.deck"},
	}}
	base := DeckConfig{Swipe: SwipeConfig{
		Right: &ActionConfig{Deck: "base-right.deck"},
	}}

	merged := MergeDeckConfig(&base, &parent)
	if merged.Swipe.Left == nil || merged.Swipe.Left.Deck != "parent-left.deck" {
		t.Errorf("swipe.left = %+v, want inherited parent-left.deck", merged.Swipe.Left)
	}
	if merged.Swipe.Right == nil || merged.Swipe.Right.Deck != "base-right.deck" {
		t.Errorf("swipe.right = %+v, want base-right.deck", merged.Swipe.Right)
	}
}

func TestLoadConfigWithoutDials(t *testing.T) {
	dc, err := LoadConfig("decks/main.deck")
	if err != nil {
		t.Fatal(err)
	}
	if len(dc.Keys) == 0 {
		t.Error("got no keys")
	}
	if len(dc.Dials) != 0 {
		t.Errorf("got %d dials, want 0", len(dc.Dials))
	}
}

// A saved config loads the same again.
func TestSaveConfig(t *testing.T) {
	config := DeckConfig{
		Background: "bg.png",
		Keys: Keys{
			{Index: 0, Widget: WidgetConfig{ID: "button", Config: map[string]interface{}{"label": "Hi"}},
				Action: &ActionConfig{Exec: "true"}},
			{Index: 1, Widget: WidgetConfig{ID: "clock"}, Color: "#ff0000"},
		},
		Dials: Dials{{Index: 0, Action: &ActionConfig{Keycode: "2"}, RingColor: "#00ff00"}},
	}
	path := filepath.Join(t.TempDir(), "saved.deck")
	if err := config.Save(path); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, config) {
		t.Errorf("loaded %+v, want %+v", got, config)
	}

	if err := config.Save(filepath.Join(t.TempDir(), "missing", "saved.deck")); err == nil {
		t.Error("expected an error saving to a missing directory")
	}
}
