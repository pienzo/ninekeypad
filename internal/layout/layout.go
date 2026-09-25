// Package layout holds keypad layouts (one layer: a name and its keys), whole-keypad
// snapshots (all layers, used for backups), the built-in presets, and the files that store
// them next to the program.
package layout

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"ninekeypad/internal/vaydeer"
)

const (
	// File format names. They say "9key" for history; files for the 4-key keypad use the
	// same formats with 4 keys per layer.
	Format       = "vaydeer-9key-layout" // one layer, in layouts/
	FormatKeypad = "vaydeer-9key-keypad" // all layers of a keypad, in backups/
	MaxLayerName = 20
	MaxLayers    = 6 // what the keypads report; the device's own value is checked when writing
)

// Layout is one keypad layer. Keys run left to right, top to bottom (key 0 = top left).
type Layout struct {
	Format  string        `json:"format"`
	Version int           `json:"version"`
	Name    string        `json:"name"`
	Note    string        `json:"note,omitempty"`
	SavedAt string        `json:"savedAt,omitempty"`
	Keys    []vaydeer.Key `json:"keys"`
}

// Keypad is everything on a keypad: its layers in order, and which one is active.
type Keypad struct {
	Format      string      `json:"format"`
	Version     int         `json:"version"`
	Note        string      `json:"note,omitempty"`
	SavedAt     string      `json:"savedAt,omitempty"`
	Device      *DeviceInfo `json:"device,omitempty"`
	ActiveLayer int         `json:"activeLayer"`
	Layers      []Layer     `json:"layers"`
}

// Layer is one layer inside a Keypad.
type Layer struct {
	Name string        `json:"name"`
	Keys []vaydeer.Key `json:"keys"`
}

// DeviceInfo records which keypad a backup came from.
type DeviceInfo struct {
	Keys       int    `json:"keys,omitempty"`
	Firmware   string `json:"firmware"`
	Bootloader string `json:"bootloader"`
	MaxLayers  int    `json:"maxLayers"`
}

func New(name string, keys []vaydeer.Key) Layout {
	return Layout{Format: Format, Version: 1, Name: name, Keys: keys}
}

func validateLayer(name string, keys []vaydeer.Key) error {
	if utf8.RuneCountInString(name) > MaxLayerName {
		return fmt.Errorf("layer name is longer than %d characters", MaxLayerName)
	}
	if err := vaydeer.CheckKeyCount(len(keys)); err != nil {
		return err
	}
	for i, k := range keys {
		if err := k.Validate(); err != nil {
			return fmt.Errorf("key %d (%s): %w", i+1, Position(i, len(keys)), err)
		}
	}
	return nil
}

func (l Layout) Validate() error {
	if l.Format != Format {
		return fmt.Errorf("not a keypad layout file (format %q)", l.Format)
	}
	return validateLayer(l.Name, l.Keys)
}

// KeyCount is the number of keys per layer (9 or 4), 0 when there are no layers.
func (k Keypad) KeyCount() int {
	if len(k.Layers) == 0 {
		return 0
	}
	return len(k.Layers[0].Keys)
}

func (k Keypad) Validate() error {
	if len(k.Layers) < 1 || len(k.Layers) > MaxLayers {
		return fmt.Errorf("a keypad holds 1 to %d layers, not %d", MaxLayers, len(k.Layers))
	}
	for i, l := range k.Layers {
		if len(l.Keys) != k.KeyCount() {
			return errors.New("all layers must have the same number of keys")
		}
		if err := validateLayer(l.Name, l.Keys); err != nil {
			return fmt.Errorf("layer %d (%s): %w", i+1, l.Name, err)
		}
	}
	return nil
}

// FromSnapshot turns what was read from a keypad into a Keypad.
func FromSnapshot(s vaydeer.Snapshot) Keypad {
	k := Keypad{
		Format: FormatKeypad, Version: 1, ActiveLayer: s.Info.ActiveLayer,
		Device: &DeviceInfo{Keys: s.Info.Keys, Firmware: s.Info.FirmwareString(), Bootloader: s.Info.BootloaderString(), MaxLayers: s.Info.MaxLayers},
	}
	for _, l := range s.Layers {
		k.Layers = append(k.Layers, Layer{Name: l.Name, Keys: append([]vaydeer.Key(nil), l.Keys...)})
	}
	return k
}

// VaydeerLayers turns a Keypad into what the protocol writes.
func (k Keypad) VaydeerLayers() []vaydeer.Layer {
	out := make([]vaydeer.Layer, len(k.Layers))
	for i, l := range k.Layers {
		out[i] = vaydeer.Layer{Name: l.Name, Keys: l.Keys}
	}
	return out
}

// Position names a key by where it sits: "top left", "center", ... on the 3x3 keypad.
// The 4-key keypad's physical arrangement is not known here, so its keys are numbered.
func Position(i, keys int) string {
	if keys != 9 {
		return fmt.Sprintf("key %d", i+1)
	}
	rows := []string{"top", "middle", "bottom"}
	cols := []string{"left", "middle", "right"}
	if i == 4 {
		return "center"
	}
	return rows[i/3] + " " + cols[i%3]
}

func key(label string, codes ...byte) vaydeer.Key {
	kind := vaydeer.KindKey
	if len(codes) > 1 {
		kind = vaydeer.KindCombo
	}
	return vaydeer.Key{Kind: kind, Label: label, Codes: codes}
}

func none() vaydeer.Key { return vaydeer.Key{Kind: vaydeer.KindNone} }

// Presets are built in; they need no file. The page offers only those with the right number
// of keys. What the 4-key keypad ships with is not known, so it has no preset.
func Presets() map[string]Layout {
	return map[string]Layout{
		// How the 9-key keypad comes from the factory (read from one with firmware 1.1.5):
		// a number pad.
		"factory": New("Layer1", []vaydeer.Key{
			key("7", '7'), key("8", '8'), key("9", '9'),
			key("4", '4'), key("5", '5'), key("6", '6'),
			key("1", '1'), key("2", '2'), key("3", '3'),
		}),
	}
}
