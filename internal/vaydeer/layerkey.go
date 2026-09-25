package vaydeer

import (
	"fmt"
	"strconv"
	"strings"
)

// The keypad cannot switch layers by itself: the Vaydeer software does it on the PC.
// This app does the same. A layer-switch key is stored on the keypad as an empty key
// (it types nothing) whose name marks it, so the setting lives on the keypad:
//
//	"Next layer"      -> the next layer, after the last comes the first
//	"Previous layer"  -> the one before, before the first comes the last
//	"Go to layer 3"   -> layer 3
//
// While the program runs it watches key presses and switches when such a key is pressed.

const (
	LayerNext = "next"
	LayerPrev = "prev"

	labelNext = "Next layer"
	labelPrev = "Previous layer"
	labelGoTo = "Go to layer "
)

// LayerLabel is the name stored on the keypad for a layer-switch target.
func LayerLabel(target string) string {
	switch target {
	case LayerNext:
		return labelNext
	case LayerPrev:
		return labelPrev
	}
	if n, err := strconv.Atoi(target); err == nil && n >= 1 && n <= 6 && target == strconv.Itoa(n) {
		return labelGoTo + target
	}
	return "" // "01", "+1", "7" ... are not targets
}

func parseLayerLabel(label string) (string, bool) {
	switch label {
	case labelNext:
		return LayerNext, true
	case labelPrev:
		return LayerPrev, true
	}
	if rest, ok := strings.CutPrefix(label, labelGoTo); ok {
		if n, err := strconv.Atoi(rest); err == nil && n >= 1 && n <= 6 && rest == strconv.Itoa(n) {
			return rest, true
		}
	}
	return "", false
}

// LayerKey makes a layer-switch key for a target ("next", "prev", "1".."6").
func LayerKey(target string) (Key, error) {
	label := LayerLabel(target)
	if label == "" {
		return Key{}, fmt.Errorf("unknown layer target %q", target)
	}
	return Key{Kind: KindLayer, Label: label, Layer: target}, nil
}

// SwitchTarget is the layer (0-based) a layer-switch key leads to from layer current,
// with count layers on the keypad; ok is false when it leads nowhere.
func SwitchTarget(target string, current, count int) (int, bool) {
	if count < 2 {
		return 0, false
	}
	switch target {
	case LayerNext:
		return (current + 1) % count, true
	case LayerPrev:
		return (current + count - 1) % count, true
	}
	n, err := strconv.Atoi(target)
	if err != nil || n < 1 || n > count || n-1 == current {
		return 0, false
	}
	return n - 1, true
}
