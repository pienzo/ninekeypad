package vaydeer

import "testing"

func TestLayerKeyIsStoredAsEmptyKeyWithName(t *testing.T) {
	for target, label := range map[string]string{"next": "Next layer", "prev": "Previous layer", "3": "Go to layer 3"} {
		k, err := LayerKey(target)
		if err != nil || k.Label != label {
			t.Fatalf("%s: %+v %v", target, k, err)
		}
		typ, sub, data, err := k.Encode()
		if err != nil || typ != 0 || sub != 0xFF || len(data) != 1 || data[0] != 0 {
			t.Fatalf("%s encodes as %d %X % X %v", target, typ, sub, data, err)
		}
		back := DecodeKey(0, 0xFF, 0, label, []byte{0})
		if !back.Same(k) {
			t.Fatalf("%s reads back as %+v", target, back)
		}
	}
}

func TestLayerKeyLabelsAreExact(t *testing.T) {
	for _, label := range []string{"next layer", "Go to layer 7", "Go to layer 03", "Go to layer ", "Layer 2"} {
		if k := DecodeKey(0, 0xFF, 0, label, []byte{0}); k.Kind != KindNone {
			t.Errorf("%q read as %+v", label, k)
		}
	}
	// A key that types something is never a layer key, whatever its name.
	if k := DecodeKey(0, 0xFF, 0, "Next layer", []byte{'4'}); k.Kind != KindKey {
		t.Errorf("typing key read as %+v", k)
	}
	for _, bad := range []string{"9", "0", "01", "+1", ""} {
		if _, err := LayerKey(bad); err == nil {
			t.Errorf("made a key for layer %q", bad)
		}
	}
	// An empty key may not borrow the reserved name, and a layer key must keep it.
	if (Key{Kind: KindNone, Label: "Next layer"}).Validate() == nil {
		t.Error("empty key with reserved name accepted")
	}
	if (Key{Kind: KindLayer, Label: "Next layer", Layer: "prev"}).Validate() == nil {
		t.Error("layer key with mismatched name accepted")
	}
}

func TestSwitchTarget(t *testing.T) {
	cases := []struct {
		target         string
		current, count int
		want           int
		ok             bool
	}{
		{"next", 0, 2, 1, true},
		{"next", 1, 2, 0, true},
		{"prev", 0, 3, 2, true},
		{"prev", 2, 3, 1, true},
		{"2", 0, 3, 1, true},
		{"2", 1, 3, 0, false}, // already there
		{"5", 0, 3, 0, false}, // no such layer
		{"next", 0, 1, 0, false},
	}
	for _, c := range cases {
		got, ok := SwitchTarget(c.target, c.current, c.count)
		if got != c.want || ok != c.ok {
			t.Errorf("%s from %d of %d: got %d %v", c.target, c.current, c.count, got, ok)
		}
	}
}
