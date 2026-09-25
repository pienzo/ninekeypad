package app

import (
	"testing"

	"ninekeypad/internal/layout"
	"ninekeypad/internal/vaydeer"
)

func TestLayerSwitchLookup(t *testing.T) {
	s := New(layout.Store{}, NewToken())
	f := layout.Presets()["factory"]
	a := append([]vaydeer.Key(nil), f.Keys...)
	b := append([]vaydeer.Key(nil), f.Keys...)
	next, _ := vaydeer.LayerKey("next")
	goTo1, _ := vaydeer.LayerKey("1")
	a[4] = next  // layer 1: center key = next layer
	b[0] = goTo1 // layer 2: top-left key = go to layer 1
	s.learn(layout.Keypad{Layers: []layout.Layer{{Name: "A", Keys: a}, {Name: "B", Keys: b}}})

	cases := []struct {
		ev   vaydeer.KeyEvent
		want int
		ok   bool
	}{
		{vaydeer.KeyEvent{Layer: 0, Key: 4, Down: true}, 1, true},
		{vaydeer.KeyEvent{Layer: 0, Key: 4, Down: false}, 0, false}, // release does nothing
		{vaydeer.KeyEvent{Layer: 0, Key: 0, Down: true}, 0, false},  // "7" types, no switch
		{vaydeer.KeyEvent{Layer: 1, Key: 0, Down: true}, 0, true},
		{vaydeer.KeyEvent{Layer: 1, Key: 4, Down: true}, 0, false},  // "5" on layer 2
		{vaydeer.KeyEvent{Layer: 5, Key: 4, Down: true}, 0, false},  // unknown layer
		{vaydeer.KeyEvent{Layer: 0, Key: 12, Down: true}, 0, false}, // no such key
	}
	for _, c := range cases {
		got, _, ok := s.layerSwitch(c.ev)
		if got != c.want || ok != c.ok {
			t.Errorf("%+v: got %d %v, want %d %v", c.ev, got, ok, c.want, c.ok)
		}
	}
}
