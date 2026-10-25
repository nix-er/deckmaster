package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// iconExts are the icon file types, in the order they're looked for.
var iconExts = []string{".svg", ".png"}

// isIconName reports whether an icon setting names an icon in the icon
// directories rather than a file: it has no file extension and isn't an
// absolute, home or explicitly relative path.
func isIconName(icon string) bool {
	return filepath.Ext(icon) == "" && !filepath.IsAbs(icon) &&
		!strings.HasPrefix(icon, "~") && !strings.HasPrefix(icon, ".")
}

// iconRoots returns the directories icons are looked up in:
// $DECKMASTER_ICON_PATH, then deckmaster/icons in $XDG_DATA_HOME and each of
// $XDG_DATA_DIRS.
func iconRoots() []string {
	roots := filepath.SplitList(os.Getenv("DECKMASTER_ICON_PATH"))

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := expandHome("~/.local/share"); err == nil {
			dataHome = home
		}
	}
	if dataHome != "" {
		roots = append(roots, filepath.Join(dataHome, "deckmaster", "icons"))
	}

	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, dir := range filepath.SplitList(dataDirs) {
		roots = append(roots, filepath.Join(dir, "deckmaster", "icons"))
	}

	var nonEmpty []string
	for _, root := range roots {
		if root != "" {
			nonEmpty = append(nonEmpty, root)
		}
	}
	return nonEmpty
}

// findIcon returns the path of the named icon. A name is either "firefox",
// searched for in every icon pack, or "simple-icons/firefox", searched for in
// that pack only. Packs are the directories in the icon roots; they may keep
// their icons in an icons subdirectory, like unpacked Stream Deck icon packs.
func findIcon(name string) (string, error) {
	pack, icon := "", name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		pack, icon = name[:i], name[i+1:]
	}
	if icon == "" {
		return "", fmt.Errorf("invalid icon name %q", name)
	}

	roots := iconRoots()
	for _, root := range roots {
		var dirs []string
		if pack != "" {
			dirs = []string{filepath.Join(root, pack)}
		} else {
			dirs = append([]string{root}, packDirs(root)...)
		}

		for _, dir := range dirs {
			if path, ok := iconInDir(dir, icon); ok {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("icon %q not found in %s", name, strings.Join(roots, ", "))
}

// packDirs returns the icon pack directories in root, sorted by name.
func packDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, e := range entries {
		path := filepath.Join(root, e.Name())
		// follow symlinks, e.g. packs linked in from elsewhere
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			dirs = append(dirs, path)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// iconInDir returns the path of the icon in a pack directory, or its icons
// subdirectory.
func iconInDir(dir, icon string) (string, bool) {
	for _, d := range []string{dir, filepath.Join(dir, "icons")} {
		for _, ext := range iconExts {
			path := filepath.Join(d, icon+ext)
			if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
				return path, true
			}
		}
	}
	return "", false
}
