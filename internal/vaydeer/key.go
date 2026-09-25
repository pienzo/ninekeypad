package vaydeer

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Key kinds this app can write. Everything else read from a keypad is kept as "other"
// and shown, but never written: its byte format is not verified.
const (
	KindNone  = "none"  // key does nothing
	KindKey   = "key"   // one key, e.g. "4"
	KindCombo = "combo" // keys pressed together, e.g. Shift+F
	KindLayer = "layer" // switches the keypad's layer; see layerkey.go
	KindOther = "other" // text, macro, mouse, app launch ... read-only here
)

const MaxLabel = 20

// Key is one key assignment.
type Key struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Codes Bytes  `json:"codes,omitempty"` // Windows virtual-key codes, modifiers first
	Layer string `json:"layer,omitempty"` // kind "layer": "next", "prev" or "1".."6"

	// Raw header and data of a key of kind "other", for display and backups.
	RawType byte  `json:"rawType,omitempty"`
	RawSub  byte  `json:"rawSub,omitempty"`
	RawEnc  byte  `json:"rawEnc,omitempty"`
	RawData Bytes `json:"rawData,omitempty"`
}

// Bytes is written to JSON as a list of numbers ([16, 70]) instead of base64, so files stay readable.
type Bytes []byte

func (b Bytes) MarshalJSON() ([]byte, error) {
	n := make([]int, len(b))
	for i, v := range b {
		n[i] = int(v)
	}
	return json.Marshal(n)
}

func (b *Bytes) UnmarshalJSON(data []byte) error {
	var n []int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	out := make(Bytes, len(n))
	for i, v := range n {
		if v < 0 || v > 255 {
			return fmt.Errorf("key code %d is out of range 0-255", v)
		}
		out[i] = byte(v)
	}
	*b = out
	return nil
}

// DecodeKey turns a read-back header and data into a Key.
func DecodeKey(keyType, sub, enc byte, label string, data []byte) Key {
	switch {
	case keyType == 0 && (len(data) == 0 || (len(data) == 1 && data[0] == 0)):
		if target, ok := parseLayerLabel(label); ok {
			return Key{Kind: KindLayer, Label: label, Layer: target}
		}
		return Key{Kind: KindNone, Label: label}
	case keyType == 0 && len(data) == 1:
		return Key{Kind: KindKey, Label: label, Codes: []byte{data[0]}}
	case keyType == 1 && len(data) >= 2:
		return Key{Kind: KindCombo, Label: label, Codes: append([]byte(nil), data...)}
	}
	return Key{Kind: KindOther, Label: label, RawType: keyType, RawSub: sub, RawEnc: enc, RawData: append([]byte(nil), data...)}
}

// Encode gives the key type, sub type and data bytes that WriteKey sends.
func (k Key) Encode() (keyType, sub byte, data []byte, err error) {
	if err := k.Validate(); err != nil {
		return 0, 0, nil, err
	}
	switch k.Kind {
	case KindNone, KindLayer: // a layer key sends nothing; its label marks it
		return 0, 0xFF, []byte{0}, nil
	case KindKey:
		return 0, 0xFF, []byte{k.Codes[0]}, nil
	case KindCombo:
		return 1, 0xFF, append([]byte(nil), k.Codes...), nil
	}
	return 0, 0, nil, errors.New("this kind of key cannot be written by this app")
}

func (k Key) Validate() error {
	if utf8.RuneCountInString(k.Label) > MaxLabel {
		return fmt.Errorf("name %q is longer than %d characters", k.Label, MaxLabel)
	}
	switch k.Kind {
	case KindNone:
		if _, ok := parseLayerLabel(k.Label); ok {
			return fmt.Errorf("the name %q is kept for layer-switch keys", k.Label)
		}
		return nil
	case KindLayer:
		if _, ok := parseLayerLabel(k.Label); !ok || k.Label != LayerLabel(k.Layer) {
			return fmt.Errorf("layer-switch key has a bad target %q", k.Layer)
		}
	case KindKey:
		if len(k.Codes) != 1 || k.Codes[0] == 0 {
			return errors.New("a single key needs exactly one key")
		}
	case KindCombo:
		if len(k.Codes) < 2 || len(k.Codes) > 6 {
			return errors.New("a key combination needs 2 to 6 keys")
		}
		for _, c := range k.Codes {
			if c == 0 {
				return errors.New("a key combination contains an empty key")
			}
		}
	case KindOther:
		return errors.New("text, macro, mouse and program keys are shown but cannot be written yet")
	default:
		return fmt.Errorf("unknown key kind %q", k.Kind)
	}
	return nil
}

// Same reports whether two keys store the same thing on the keypad.
func (k Key) Same(o Key) bool {
	if k.Kind != o.Kind || k.Label != o.Label || k.Layer != o.Layer || len(k.Codes) != len(o.Codes) {
		return false
	}
	for i := range k.Codes {
		if k.Codes[i] != o.Codes[i] {
			return false
		}
	}
	return k.Kind != KindOther || (k.RawType == o.RawType && k.RawSub == o.RawSub && string(k.RawData) == string(o.RawData))
}
