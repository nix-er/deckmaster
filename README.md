# deckmaster

[![Latest Release](https://img.shields.io/github/release/muesli/deckmaster.svg?style=for-the-badge)](https://github.com/muesli/deckmaster/releases)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=for-the-badge)](https://pkg.go.dev/github.com/nix-er/deckmaster)
[![Software License](https://img.shields.io/badge/license-MIT-blue.svg?style=for-the-badge)](/LICENSE)
[![Build Status](https://img.shields.io/github/actions/workflow/status/nix-er/deckmaster/build.yml?branch=master&style=for-the-badge)](https://github.com/nix-er/deckmaster/actions)
[![Go ReportCard](https://goreportcard.com/badge/github.com/nix-er/deckmaster?style=for-the-badge)](https://goreportcard.com/report/nix-er/deckmaster)

An application to control your Elgato Stream Deck on Linux

## Features

- Multiple pages & navigation between decks
- Buttons (icons & text)
- Background images
- Brightness control
- Supports different actions for short & long presses
- Supports all Stream Deck models, including:
    - Stream Deck+ and + XL dials and touch screen
    - Stream Deck Studio dials with LED rings
    - Stream Deck Neo touch keys and info screen
    - Stream Deck Pedal
- Comes with a collection of widgets:
    - Buttons
    - Time (with formatting)
    - CPU/Mem usage
    - Weather
    - Command output
    - Recently used windows (X11-only)
- Lets you trigger several actions:
    - Run commands
    - Emulate a key-press
    - Paste to clipboard
    - Trigger a dbus call

## Supported Devices

| Device | Keys | Other controls |
|---|---|---|
| Stream Deck Original, MK.2, Module 15 Keys (all revisions) | 5×3 | |
| Stream Deck Mini, Module 6 Keys (all revisions) | 3×2 | |
| Stream Deck XL, Module 32 Keys (all revisions) | 8×4 | |
| Stream Deck+ | 4×2 | 4 dials, touch screen |
| Stream Deck + XL* | 9×4 | 6 dials, touch screen |
| Stream Deck Neo* | 4×2 | 2 touch keys with LEDs, info screen |
| Stream Deck Studio* | 16×2 (144×112 pixels) | 2 dials with LED rings |
| Stream Deck Pedal* | 3 pedals | |

\* Support for these models follows the reference implementations of their
protocol, but hasn't been tested on hardware yet. Please report how it works!

## Installation

### Packages

- Arch Linux: [deckmaster](https://aur.archlinux.org/packages/deckmaster/)
- [Packages](https://github.com/muesli/deckmaster/releases) in Alpine, Debian & RPM formats
- [Binaries](https://github.com/muesli/deckmaster/releases) for various architectures

### From source

Make sure you have a working Go environment (Go 1.26 or higher is required).
See the [install instructions](https://golang.org/doc/install.html).

To install deckmaster, simply run:

    git clone https://github.com/nix-er/deckmaster.git
    cd deckmaster
    go build

Or install it directly with:

    go install github.com/nix-er/deckmaster@latest

Building requires cgo (a C compiler) for USB access.

### Nix

The repository is a Nix flake. To run deckmaster without installing it:

    nix run github:nix-er/deckmaster -- -deck main.deck

To add it to a NixOS configuration, add the flake as an input and use its
package. The package also ships the udev rules described below:

```nix
{
  inputs.deckmaster.url = "github:nix-er/deckmaster";

  # in your NixOS module:
  environment.systemPackages = [ inputs.deckmaster.packages.${pkgs.stdenv.hostPlatform.system}.default ];
  services.udev.packages = [ inputs.deckmaster.packages.${pkgs.stdenv.hostPlatform.system}.default ];
}
```

`nix develop` opens a shell with Go, a C compiler, golangci-lint and goreleaser.

## System Setup

On Linux you need to set up some `udev` rules to be able to access the device as
a regular user. Copy [udev/70-streamdeck.rules](udev/70-streamdeck.rules) to
`/etc/udev/rules.d/` (on NixOS, use `services.udev.packages` as shown above):

    sudo cp udev/70-streamdeck.rules /etc/udev/rules.d/
    sudo udevadm control --reload-rules
    sudo udevadm trigger

The rules use `TAG+="uaccess"`, which gives the user logged in on the local
seat access to the device, so no extra group membership is needed. The file name
must sort before `73-seat-late.rules`, or `uaccess` has no effect. Unplug and
re-plug the device, and you should be good to go.

To emulate key presses, deckmaster also needs write access to `/dev/uinput`;
see the commented rule at the end of the file.

### Starting deckmaster automatically

If you want deckmaster to be started automatically upon device plugin, you can
use systemd path activation, adding `streamdeck.path` and `streamdeck.service`
files to `$HOME/.config/systemd/user`.

`streamdeck.path` contents:

```ini
[Unit]
Description="Stream Deck Device Path"

[Path]
# the device name will be different if you use streamdeck-mini or streamdeck-xl
PathExists=/dev/streamdeck
Unit=streamdeck.service

[Install]
WantedBy=multi-user.target
```

`streamdeck.service` contents:

```ini
[Unit]
Description=Deckmaster Service

[Service]
# adjust the path to deckmaster and .deck file to suit your needs
ExecStart=/usr/local/bin/deckmaster --deck path-to/some.deck
Restart=on-failure
ExecReload=kill -HUP $MAINPID

[Install]
WantedBy=default.target
```

Then enable and start the `streamdeck.path` unit:

```bash
systemctl --user enable streamdeck.path
systemctl --user start streamdeck.path
```

## Usage

Start `deckmaster` with the initial deck configuration you want to load:

```bash
deckmaster -deck deck/main.deck
```

You can control the brightness, in percent:

```bash
deckmaster -brightness 50
```

Control a specific streamdeck:

```bash
deckmaster -device [serial number]
```

Set a sleep timeout after which the screen gets turned off:

```bash
deckmaster -sleep 10m
```

## Configuration

You can find a few example configurations in the [decks](https://github.com/nix-er/deckmaster/tree/master/decks)
directory. Edit them to your needs!

### Widgets

Any widget is build up the following way:

```toml
[[keys]]
  index = 0
```

`index` needs to be present in every widget and describes the position of the
widget on the streamdeck. `index` is 0-indexed and counted from top to bottom
and left to right.

#### Update interval for widgets

Optionally, you can configure an update `interval` for each widget:

```toml
[keys.widget]
  id = "button"
  interval = 500 # optional
```

The attribute `interval` defines the time in `ms` between two consecutive
updates of a widget.

#### Button

A simple button that can display an image and/or a label.

```toml
[keys.widget]
  id = "button"
  [keys.widget.config]
    icon = "/some/image.png" # optional
    label = "My Button" # optional
    fontsize = 10.0 # optional
    color = "#fefefe" # optional
    flatten = true # optional
```

If `flatten` is `true` all opaque pixels of the icon will have the color `color`.

#### Recent Window (requires X11)

Displays the icon of a recently used window/application. Pressing the button
activates the window.

```toml
[keys.widget]
  id = "recentWindow"
  [keys.widget.config]
    window = 1
    showTitle = true # optional
```

If `showTitle` is `true`, the title of the window will be displayed below the
window icon.

#### Time

A flexible widget that can display the current time or date.

```toml
[keys.widget]
  id = "time"
  [keys.widget.config]
    format = "%H;%i;%s"
    font = "bold;regular;thin" # optional
    color = "#fefefe" # optional
    layout = "0x0+72x24;0x24+72x24;0x48+72x24" # optional
```

With `layout` custom layouts can be definded in the format `[posX]x[posY]+[width]x[height]`.

Values for `format` are:

| %   | gets replaced with                                                 |
| --- | ------------------------------------------------------------------ |
| %Y  | A full numeric representation of a year, 4 digits                  |
| %y  | A two digit representation of a year                               |
| %F  | A full textual representation of a month, such as January or March |
| %M  | A short textual representation of a month, three letters           |
| %m  | Numeric representation of a month, with leading zeros              |
| %l  | A full textual representation of the day of the week               |
| %D  | A textual representation of a day, three letters                   |
| %d  | Day of the month, 2 digits with leading zeros                      |
| %h  | 12-hour format of an hour with leading zeros                       |
| %H  | 24-hour format of an hour with leading zeros                       |
| %i  | Minutes with leading zeros                                         |
| %s  | Seconds with leading zeros                                         |
| %a  | Lowercase Ante meridiem and Post meridiem                          |
| %t  | Timezone abbreviation                                              |

#### Top

This widget shows the current CPU or memory utilization as a bar graph.

```toml
[keys.widget]
  id = "top"
  [keys.widget.config]
    mode = "cpu"
    color = "#fefefe" # optional
    fillColor = "#d497de" # optional
```

There are two values for `mode`: `cpu` and `memory`.

#### Command

A widget that displays the output of commands.

```toml
[keys.widget]
  id = "command"
  [keys.widget.config]
    command = "echo 'Files:'; ls -a ~ | wc -l"
    font = "regular;bold" # optional
    color = "#fefefe" # optional
    layout = "0x0+72x20;0x20+72x52" # optional
```

#### Weather

A widget that displays the weather condition and temperature.

```toml
[keys.widget]
  id = "weather"
  [keys.widget.config]
    location = "MyCity" # optional
    unit = "celsius" # optional
    color = "#fefefe" # optional
    flatten = true # optional
    theme = "openmoji" # optional
```

The supported location types can be found [here](http://wttr.in/:help). The unit
has to be either `celsius` or `fahrenheit`. If `flatten` is `true` all opaque
pixels of the condition icon will have the color `color`. In case `theme` is set
corresponding icons with correct names need to be placed in
`~/.local/share/deckmaster/themes/[theme]`. The default icons with their
respective names can be found [here](https://github.com/nix-er/deckmaster/tree/master/assets/weather).

### Actions

You can hook up any key with several actions. A regular keypress will trigger
the widget's configured `keys.action`, while holding the key will trigger
`keys.action_hold`.

#### Switch deck

```toml
[keys.action]
  deck = "relative/path/to/another.deck"
```

#### Run a command

```toml
[keys.action]
  exec = "some_command --with-parameters"
```

#### Emulate key-presses

```toml
[keys.action]
  keycode = "Leftctrl-C"
```

Emulate a series of key-presses with delay in between:

```toml
[keys.action]
  keycode = "Leftctrl-X+500 / Leftctrl-V / Num1"
```

A list of available `keycodes` can be found here: [keycodes](https://github.com/nix-er/deckmaster/blob/master/keycodes.go)

#### Paste to clipboard

```toml
[keys.action]
  paste = "some text"
```

#### Trigger a dbus call

```toml
[keys.action]
  [dbus]
    object = "object"
    path = "path"
    method = "method"
    value = "value"
```

#### Device actions

Increase the brightness. If no value is specified, it will be increased by 10%:

```toml
[keys.action]
  device = "brightness+5"
```

Decrease the brightness. If no value is specified, it will be decreased by 10%:

```toml
[keys.action]
  device = "brightness-5"
```

Set the brightness to a specific value between 0 and 100:

```toml
[keys.action]
  device = "brightness=50"
```

Put the device into sleep mode, blanking the screen until the next key gets
pressed:

```toml
[keys.action]
  device = "sleep"
```

### Background Image

You can configure each deck to display an individual wallpaper behind its
widgets:

```toml
background = "/some/image.png"
```

### Re-using another deck's configuration

If you specify a `parent` inside a deck's configuration, it will inherit all
of the parent's settings that are not overwritten by the deck's own settings.
This even works recursively:

```toml
parent = "another.deck"
```

Dials, swipe actions and the info screen widget are inherited the same way.

### Dials and touch screen

The Stream Deck+ (four dials) and + XL (six dials) have a touch screen segment
above each dial. A dial and its segment are configured together in a `[[dials]]` entry,
with `index` 0 being the leftmost dial. All settings are optional:

```toml
[[dials]]
  index = 0
  # widget shown on the touch screen segment above the dial
  [dials.widget]
    id = "button"
    [dials.widget.config]
      icon = "/some/volume.png"
      label = "Volume"
  # pressing the dial (action_hold for long presses)
  [dials.action]
    keycode = "Mute"
  # turning the dial counterclockwise / clockwise
  [dials.action_left]
    keycode = "VolumeDown"
  [dials.action_right]
    keycode = "VolumeUp"
  # tapping the segment (action_touch_hold for long presses)
  [dials.action_touch]
    exec = "pavucontrol"
```

All widgets and actions work on dials. Buttons, `time` and `command` widgets
use the whole segment (a button shows its icon left of its label); the others
are drawn as a square in the middle of their segment. The touch screen can't
show fine detail, so large icons and short labels work best, e.g. a single
line `format = "%H:%i"` for a clock. A fast turn sends several steps at once;
the turn action then runs once per step (at most 10 times), except for deck
switches.
Tapping a segment without an `action_touch` triggers the widget's own action
(e.g. activating a window for `recentWindow`).

Swiping across the touch screen can trigger an action, e.g. to switch decks:

```toml
[swipe]
  [swipe.left]
    deck = "next.deck"
  [swipe.right]
    deck = "previous.deck"
```

See [decks/plus.deck](decks/plus.deck) for a complete example.

### Stream Deck Studio dial LEDs

The Stream Deck Studio's two dials have no screen, but an LED ring and a knob
LED. Configure them in the `[[dials]]` entry, next to the dial's actions. The
ring can show a value from 0 to 100, printed by a command:

```toml
[[dials]]
  index = 0
  ring_color = "#00c060"
  knob_color = "#ffffff"
  # optional: light only part of the ring, e.g. to show the volume
  ring_value = "pamixer --get-volume"
  ring_interval = 1000 # milliseconds, optional
```

The value command runs every `ring_interval` milliseconds (every second by
default), and right after the dial is turned or pressed. LEDs without a color
are turned off. See [decks/studio.deck](decks/studio.deck) for a complete
example.

### Stream Deck Neo touch keys and info screen

The Stream Deck Neo's two touch keys have no display, but an LED. They are
keys 8 and 9, configured like the other keys with a `color` instead of a
widget:

```toml
[[keys]]
  index = 8
  color = "#ff8000"
  [keys.action]
    keycode = "VolumeDown"
```

Any widget can be shown on the info screen between them (248×58 pixels):

```toml
[info.widget]
  id = "time"
  [info.widget.config]
    format = "%H:%i"
```

See [decks/neo.deck](decks/neo.deck) for a complete example.

### Stream Deck Pedal

The pedals are keys 0 to 2 (left to right). As they have no display, only
their actions are used.

## More Decks!

* [deckmaster-emojis](https://github.com/muesli/deckmaster-emojis), an Emoji keyboard deck
* [deckmaster-helldivers2](https://github.com/boj/deckmaster-helldivers2), a deck for calling down Helldivers 2 game stratagems

Made your own useful decks? Submit a pull request!

## Feedback

Got some feedback or suggestions? Please open an issue or drop me a note!

* [Twitter](https://twitter.com/mueslix)
* [The Fediverse](https://mastodon.social/@fribbledom)
