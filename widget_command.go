package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// CommandWidget is a widget displaying the output of command(s).
type CommandWidget struct {
	*BaseWidget

	commands *commandRunner
	fonts    []string
	frames   []image.Rectangle
	colors   []color.Color
}

// NewCommandWidget returns a new CommandWidget.
func NewCommandWidget(bw *BaseWidget, opts WidgetConfig) (*CommandWidget, error) {
	bw.setInterval(msDuration(opts.Interval), time.Second)

	var commands, fonts, frameReps []string
	var colors []color.Color
	r := configReader{config: opts.Config}
	r.read("command", &commands)
	r.read("font", &fonts)
	r.read("layout", &frameReps)
	r.read("color", &colors)
	if r.err != nil {
		return nil, r.err
	}

	layout := NewLayout(bw.canvas())
	frames := layout.FormatLayout(frameReps, len(commands))

	for i := 0; i < len(commands); i++ {
		if len(fonts) < i+1 {
			fonts = append(fonts, "regular")
		}
		if len(colors) < i+1 {
			colors = append(colors, DefaultColor)
		}
	}

	return &CommandWidget{
		BaseWidget: bw,
		commands:   newCommandRunner(fmt.Sprintf("key %d", bw.key), commands),
		fonts:      fonts,
		frames:     frames,
		colors:     colors,
	}, nil
}

// RequiresUpdate returns true when the widget wants to be repainted.
func (w *CommandWidget) RequiresUpdate() bool {
	return w.commands.Fresh() || w.BaseWidget.RequiresUpdate()
}

// Update renders the widget.
func (w *CommandWidget) Update() error {
	outputs := w.commands.Outputs()

	img := image.NewRGBA(w.canvas())
	for i, str := range outputs {
		if str == "" {
			continue
		}
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

// commandRunner runs commands in the background, so slow commands don't
// block the event loop, and keeps their latest output.
type commandRunner struct {
	name     string
	commands []string

	mu      sync.Mutex
	outputs []string
	errs    []string
	running bool
	fresh   bool
}

// newCommandRunner returns a commandRunner for the given commands. The name
// is used in error messages.
func newCommandRunner(name string, commands []string) *commandRunner {
	return &commandRunner{
		name:     name,
		commands: commands,
		outputs:  make([]string, len(commands)),
		errs:     make([]string, len(commands)),
	}
}

// Fresh returns true when new output is available.
func (r *commandRunner) Fresh() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.fresh
}

// Outputs returns the latest output of each command, empty until a command
// first succeeds. Unless it's called because new output is available, it
// also runs the commands again in the background, if they aren't running
// already.
func (r *commandRunner) Outputs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.fresh && !r.running {
		r.running = true
		go r.run()
	}
	r.fresh = false
	return slices.Clone(r.outputs)
}

// run runs the commands. A failing command keeps its previous output, and
// its error is only logged when it changes, not on every run.
func (r *commandRunner) run() {
	outputs := make([]string, len(r.commands))
	errs := make([]string, len(r.commands))
	for i, command := range r.commands {
		out, err := runCommand(command)
		if err != nil {
			errs[i] = err.Error()
			continue
		}
		outputs[i] = out
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.commands {
		if errs[i] != "" {
			if errs[i] != r.errs[i] {
				fmt.Fprintf(os.Stderr, "Command %q of %s failed: %s\n", r.commands[i], r.name, errs[i])
			}
		} else {
			r.outputs[i] = outputs[i]
		}
	}
	r.errs = errs
	r.running = false
	r.fresh = true
}

func runCommand(command string) (string, error) {
	output, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(output), "\n"), nil
}
