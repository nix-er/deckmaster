package main

import (
	"reflect"
	"testing"
	"time"
)

type trigger struct {
	index uint8
	hold  bool
}

// pressTrackerRecorder returns a pressTracker recording its triggers.
func pressTrackerRecorder() (*pressTracker, chan func(), *[]trigger) {
	var triggers []trigger
	holds := make(chan func(), 8)
	p := newPressTracker("key", holds, func(index uint8, hold bool) {
		triggers = append(triggers, trigger{index, hold})
	})
	return p, holds, &triggers
}

// runHolds runs the long press checks arriving within the long press
// duration plus some slack, like the event loop does.
func runHolds(holds chan func()) {
	timeout := time.After(longPressDuration + 200*time.Millisecond)
	for {
		select {
		case hold := <-holds:
			hold()
		case <-timeout:
			return
		}
	}
}

func checkTriggers(t *testing.T, got *[]trigger, want ...trigger) {
	t.Helper()
	if len(*got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("got triggers %v, want %v", *got, want)
	}
}

func TestPressTrackerShortPress(t *testing.T) {
	p, holds, triggers := pressTrackerRecorder()

	p.handle(3, true)
	p.handle(3, false)
	runHolds(holds)

	checkTriggers(t, triggers, trigger{3, false})
}

func TestPressTrackerLongPress(t *testing.T) {
	p, holds, triggers := pressTrackerRecorder()

	p.handle(3, true)
	runHolds(holds)
	p.handle(3, false)

	checkTriggers(t, triggers, trigger{3, true})
}

// Quick taps followed by a long press must trigger a single long press: the
// timers of the taps must not count the later press as theirs. Seen on the
// Stream Deck+, where a long press putting the device to sleep ran three
// times and it woke up with brightness 0.
func TestPressTrackerTapsThenLongPress(t *testing.T) {
	p, holds, triggers := pressTrackerRecorder()

	for range 2 {
		p.handle(0, true)
		time.Sleep(20 * time.Millisecond)
		p.handle(0, false)
		time.Sleep(20 * time.Millisecond)
	}
	p.handle(0, true)
	runHolds(holds)
	p.handle(0, false)

	checkTriggers(t, triggers, trigger{0, false}, trigger{0, false}, trigger{0, true})
}

// A long press must still work after a release went missing, e.g. because it
// only woke the device from sleep (upstream #79).
func TestPressTrackerMissedRelease(t *testing.T) {
	p, holds, triggers := pressTrackerRecorder()

	p.handle(3, true)
	runHolds(holds)
	// the release is never reported

	p.handle(3, true)
	runHolds(holds)
	p.handle(3, false)

	checkTriggers(t, triggers, trigger{3, true}, trigger{3, true})
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	for path, want := range map[string]string{
		"~":          "/home/test",
		"~/a/b.deck": "/home/test/a/b.deck",
		"a/~/b":      "a/~/b",
		"/abs":       "/abs",
		"":           "",
	} {
		got, err := expandHome(path)
		if err != nil || got != want {
			t.Errorf("expandHome(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
	if _, err := expandHome("~user/x"); err == nil {
		t.Error("expandHome(\"~user/x\") succeeded, want an error")
	}
}
