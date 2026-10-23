// Deckmaster is an application to control Elgato Stream Deck devices on
// Linux.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/bendahl/uinput"
	"github.com/godbus/dbus/v5"
	"github.com/mitchellh/go-homedir"
	"github.com/nix-er/streamdeck"
)

var (
	// Version contains the application version number. It's set via ldflags
	// when building.
	Version = ""

	// CommitSHA contains the SHA of the commit that this application was built
	// against. It's set via ldflags when building.
	CommitSHA = ""

	deck *Deck

	dbusConn *dbus.Conn
	keyboard uinput.Keyboard
	shutdown = make(chan error)

	xorg          *Xorg
	recentWindows []Window

	deckFile   = flag.String("deck", "main.deck", "path to deck config file")
	device     = flag.String("device", "", "which device to use (serial number)")
	brightness = flag.Uint("brightness", 80, "brightness in percent")
	sleep      = flag.String("sleep", "", "sleep timeout")
	verbose    = flag.Bool("verbose", false, "verbose output")
	version    = flag.Bool("version", false, "display version")
)

const (
	fadeDuration      = 250 * time.Millisecond
	longPressDuration = 350 * time.Millisecond
)

func fatal(v ...interface{}) {
	go func() { shutdown <- errors.New(fmt.Sprint(v...)) }()
}

func fatalf(format string, a ...interface{}) {
	go func() { shutdown <- fmt.Errorf(format, a...) }()
}

func verbosef(format string, a ...interface{}) {
	if !*verbose {
		return
	}

	fmt.Printf(format+"\n", a...)
}

func expandPath(base, path string) (string, error) {
	var err error
	path, err = homedir.Expand(path)
	if err != nil {
		return "", err
	}
	if base == "" {
		return path, nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}

	return filepath.Abs(path)
}

func eventLoop(dev *streamdeck.Device, tch chan interface{}) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)

	// long presses are detected on separate goroutines, but their actions run
	// here, so only this goroutine writes to the device
	holds := make(chan func(), 8)

	keys := newPressTracker("key", holds, func(index uint8, hold bool) {
		deck.triggerAction(dev, index, hold)
	})
	dials := newPressTracker("dial", holds, func(index uint8, hold bool) {
		deck.triggerDialAction(dev, index, hold)
	})

	ich, err := dev.ReadInput()
	if err != nil {
		return err
	}
	for {
		select {
		case <-time.After(100 * time.Millisecond):
			deck.updateWidgets()

		case e, ok := <-ich:
			if !ok {
				if err = dev.Open(); err != nil {
					return err
				}
				if ich, err = dev.ReadInput(); err != nil {
					return err
				}
				continue
			}

			switch e := e.(type) {
			case streamdeck.Key:
				keys.handle(e.Index, e.Pressed)

			case streamdeck.DialPress:
				dials.handle(e.Index, e.Pressed)

			case streamdeck.DialTurn:
				verbosef("Dial %d turned %d step(s)", e.Index, e.Steps)
				deck.turnDial(dev, e.Index, e.Steps)

			case streamdeck.Touch:
				handleTouch(dev, e)
			}

		case hold := <-holds:
			hold()

		case e := <-tch:
			switch event := e.(type) {
			case WindowClosedEvent:
				handleWindowClosed(event)

			case ActiveWindowChangedEvent:
				handleActiveWindowChanged(dev, event)
			}

		case err := <-shutdown:
			return err

		case <-hup:
			verbosef("Received SIGHUP, reloading configuration...")

			nd, err := LoadDeck(dev, ".", deck.File)
			if err != nil {
				verbosef("The new configuration is not valid, keeping the current one.")
				fmt.Fprintf(os.Stderr, "Configuration Error: %s\n", err)
				continue
			}

			deck = nd
			deck.updateWidgets()

		case <-sigs:
			fmt.Println("Shutting down...")
			return nil
		}
	}
}

// minSwipeDistance is the minimum horizontal distance in pixels for a swipe to
// trigger an action.
const minSwipeDistance = 100

// pressTracker turns press/release events of keys or dials into short and long
// press actions.
type pressTracker struct {
	name       string
	states     sync.Map
	timestamps map[uint8]time.Time
	holds      chan<- func()
	trigger    func(index uint8, hold bool)
}

// newPressTracker returns a pressTracker calling trigger for short presses
// directly, and sending long presses to holds.
func newPressTracker(name string, holds chan<- func(), trigger func(index uint8, hold bool)) *pressTracker {
	return &pressTracker{
		name:       name,
		timestamps: make(map[uint8]time.Time),
		holds:      holds,
		trigger:    trigger,
	}
}

// handle processes a press or release of the key or dial with the given index.
func (p *pressTracker) handle(index uint8, pressed bool) {
	var state bool
	if s, ok := p.states.Load(index); ok {
		state = s.(bool)
	}
	p.states.Store(index, pressed)

	if state && !pressed {
		// was released
		if time.Since(p.timestamps[index]) < longPressDuration {
			verbosef("Triggering short action for %s %d", p.name, index)
			p.trigger(index, false)
		}
	}
	if !state && pressed {
		// was pressed
		go func() {
			// launch timer to observe the state
			time.Sleep(longPressDuration)

			if s, ok := p.states.Load(index); ok && s.(bool) {
				// still pressed
				p.holds <- func() {
					verbosef("Triggering long action for %s %d", p.name, index)
					p.trigger(index, true)
				}
			}
		}()
	}
	p.timestamps[index] = time.Now()
}

// handleTouch handles taps, long presses and swipes on the touch screen.
func handleTouch(dev *streamdeck.Device, t streamdeck.Touch) {
	switch t.Type {
	case streamdeck.TouchTap, streamdeck.TouchLongPress:
		width := dev.ScreenSegmentWidth()
		if width == 0 {
			return
		}
		s := min(uint(t.X)/width, uint(dev.ScreenSegments)-1)
		segment := uint8(s) //nolint:gosec // s < dev.ScreenSegments

		hold := t.Type == streamdeck.TouchLongPress
		action := "tapped"
		if hold {
			action = "long-pressed"
		}
		verbosef("Touch screen segment %d %s at %d,%d", segment, action, t.X, t.Y)
		deck.touchSegment(dev, segment, hold)

	case streamdeck.TouchSwipe:
		distance := int(t.EndX) - int(t.X)
		verbosef("Touch screen swiped from %d,%d to %d,%d", t.X, t.Y, t.EndX, t.EndY)
		switch {
		case distance <= -minSwipeDistance:
			deck.swipe(dev, true)
		case distance >= minSwipeDistance:
			deck.swipe(dev, false)
		}
	}
}

func closeDevice(dev *streamdeck.Device) {
	if err := dev.Reset(); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to reset Stream Deck")
	}
	if err := dev.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to close Stream Deck")
	}
}

func initDevice() (*streamdeck.Device, error) {
	d, err := streamdeck.Devices()
	if err != nil {
		return nil, err
	}
	if len(d) == 0 {
		return nil, fmt.Errorf("no Stream Deck devices found")
	}

	dev := d[0]
	if len(*device) > 0 {
		found := false
		for _, v := range d {
			if v.Serial == *device {
				dev = v
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintln(os.Stderr, "Can't find device. Available devices:")
			for _, v := range d {
				fmt.Fprintf(os.Stderr, "Serial %s (%d buttons)\n", v.Serial, v.Keys)
			}
			os.Exit(1)
		}
	}

	if err := dev.Open(); err != nil {
		return nil, err
	}
	ver, err := dev.FirmwareVersion()
	if err != nil {
		return &dev, err
	}
	verbosef("Found device with serial %s (%d buttons, firmware %s)",
		dev.Serial, dev.Keys, ver)
	if dev.Dials > 0 || dev.ScreenWidth > 0 {
		verbosef("Device has %d dials and a %dx%d touch screen",
			dev.Dials, dev.ScreenWidth, dev.ScreenHeight)
	}

	if err := dev.Reset(); err != nil {
		return &dev, err
	}

	if *brightness > 100 {
		*brightness = 100
	}
	if err = dev.SetBrightness(uint8(*brightness)); err != nil {
		return &dev, err
	}

	dev.SetSleepFadeDuration(fadeDuration)
	if len(*sleep) > 0 {
		timeout, err := time.ParseDuration(*sleep)
		if err != nil {
			return &dev, err
		}

		dev.SetSleepTimeout(timeout)
	}

	return &dev, nil
}

func run() error {
	if err := loadFonts(); err != nil {
		return err
	}

	// initialize device
	dev, err := initDevice()
	if dev != nil {
		defer closeDevice(dev)
	}
	if err != nil {
		return fmt.Errorf("unable to initialize Stream Deck: %w", err)
	}

	// initialize dbus connection
	dbusConn, err = dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("unable to connect to dbus: %w", err)
	}

	// initialize xorg connection and track window focus
	tch := make(chan interface{})
	xorg, err = Connect(os.Getenv("DISPLAY"))
	if err == nil {
		defer xorg.Close()
		xorg.TrackWindows(tch, time.Second)
	} else {
		fmt.Fprintf(os.Stderr, "Could not connect to X server: %s\n", err)
		fmt.Fprintln(os.Stderr, "Tracking window manager will be disabled!")
	}

	// initialize virtual keyboard
	keyboard, err = uinput.CreateKeyboard("/dev/uinput", []byte("Deckmaster"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create virtual input device (/dev/uinput): %s\n", err)
		fmt.Fprintln(os.Stderr, "Emulating keyboard events will be disabled!")
	} else {
		defer keyboard.Close() //nolint:errcheck
	}

	// load deck
	deck, err = LoadDeck(dev, ".", *deckFile)
	if err != nil {
		return fmt.Errorf("can't load deck: %w", err)
	}
	deck.updateWidgets()

	return eventLoop(dev, tch)
}

func main() {
	flag.Parse()

	if *version {
		if len(CommitSHA) > 7 {
			CommitSHA = CommitSHA[:7]
		}
		if Version == "" {
			Version = "(built from source)"
		}

		fmt.Printf("deckmaster %s", Version)
		if len(CommitSHA) > 0 {
			fmt.Printf(" (%s)", CommitSHA)
		}

		fmt.Println()
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
