package vaydeer

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"ninekeypad/internal/vaydeer/vaydeertest"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBuildFrame(t *testing.T) {
	f, err := BuildFrame(0x60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 65 || !bytes.Equal(f[:4], []byte{0x00, 0x60, 0x00, 0x60}) {
		t.Fatalf("frame % X", f[:8])
	}
	// Read key 0 header: 62 03 FF 00 00, xor = 62^03^FF = 9E
	f, _ = BuildFrame(0x62, []byte{0xFF, 0, 0})
	if !bytes.Equal(f[:7], []byte{0x00, 0x62, 0x03, 0xFF, 0x00, 0x00, 0x9E}) {
		t.Fatalf("frame % X", f[:8])
	}
}

func TestFirmwareCommandBlocked(t *testing.T) {
	if _, err := BuildFrame(0xFC, []byte{0xFF}); err == nil {
		t.Fatal("firmware command was not blocked")
	}
}

func TestPayloadLimit(t *testing.T) {
	if _, err := BuildFrame(0x61, make([]byte, 61)); err != nil {
		t.Fatalf("61 bytes must fit: %v", err)
	}
	if _, err := BuildFrame(0x61, make([]byte, 62)); err == nil {
		t.Fatal("62 bytes must not fit")
	}
}

// Answers recorded from the tested keypad (9 keys, firmware 1.1.5).
func TestParseRecordedDeviceInfo(t *testing.T) {
	status, data, err := ParseResponse(0x60, unhex(t, "60 0C 00 01 09 01 01 05 00 02 02 00 01 06 66 00 00 00"))
	if err != nil || status != 0 {
		t.Fatalf("status %d err %v", status, err)
	}
	want := unhex(t, "01 09 01 01 05 00 02 02 00 01 06")
	if !bytes.Equal(data, want) {
		t.Fatalf("data % X", data)
	}
}

func TestParseRejectsBadChecksum(t *testing.T) {
	if _, _, err := ParseResponse(0x60, unhex(t, "60 0C 00 01 09 01 01 05 00 02 02 00 01 06 67")); err == nil {
		t.Fatal("bad checksum accepted")
	}
}

func TestParseKeyEvent(t *testing.T) {
	ev, ok := ParseKeyEvent([]byte{0xFB, 0x03, 0x00, 0x04, 0x00, 0xFB ^ 0x03 ^ 0x04, 0, 0})
	if !ok || ev.Key != 4 || !ev.Down || ev.Layer != 0 {
		t.Fatalf("%+v %v", ev, ok)
	}
	ev, ok = ParseKeyEvent([]byte{0xFB, 0x03, 0x00, 0x08, 0x02, 0xFB ^ 0x03 ^ 0x08 ^ 0x02})
	if !ok || ev.Key != 8 || ev.Down {
		t.Fatalf("%+v %v", ev, ok)
	}
	if _, ok := ParseKeyEvent([]byte{0x60, 0x03, 0, 0, 0, 0}); ok {
		t.Fatal("parsed a non-event")
	}
}

func TestReadFactoryKeypad(t *testing.T) {
	s, err := NewClient(vaydeertest.NewFactory()).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Layers) != 1 || s.Layers[0].Name != "Layer1" || s.Info.FirmwareString() != "1.1.5" {
		t.Fatalf("%+v", s)
	}
	for i, d := range "789456123" {
		want := Key{Kind: KindKey, Label: string(d), Codes: []byte{byte(d)}}
		if !s.Layers[0].Keys[i].Same(want) {
			t.Errorf("key %d = %+v", i, s.Layers[0].Keys[i])
		}
	}
}

var ratingKeys = []Key{
	{Kind: KindKey, Label: "4 stars", Codes: []byte{'4'}},
	{Kind: KindCombo, Label: "Favourite", Codes: []byte{16, 'F'}},
	{Kind: KindCombo, Label: "Bookmark", Codes: []byte{16, 'B'}},
	{Kind: KindNone}, {Kind: KindNone}, {Kind: KindNone},
	{Kind: KindKey, Label: "3 stars", Codes: []byte{'3'}},
	{Kind: KindKey, Label: "2 stars", Codes: []byte{'2'}},
	{Kind: KindKey, Label: "1 star", Codes: []byte{'1'}},
}

func digitKeys() []Key {
	keys := make([]Key, 9)
	for i, d := range "789456123" {
		keys[i] = Key{Kind: KindKey, Label: string(d), Codes: []byte{byte(d)}}
	}
	return keys
}

// This frame sequence (same key kinds) was written and checked on the tested keypad (fw 1.1.5).
func TestWriteOneLayerFrames(t *testing.T) {
	fk := vaydeertest.NewFactory()
	c := NewClient(fk)
	if err := c.WriteLayers([]Layer{{Name: "Rating", Keys: ratingKeys}}, 6); err != nil {
		t.Fatal(err)
	}
	if fk.Log[0] != "FD " || !strings.HasPrefix(fk.Log[1], "65 00 00 00 52") || fk.Log[len(fk.Log)-1] != "66 00 00" || fk.Commits != 1 {
		t.Fatalf("command order: %q ... %q", fk.Log[:2], fk.Log[len(fk.Log)-1])
	}
	// Favourite key: header, one data chunk (seq 0: Shift, F), end.
	want := []string{"61 FF 00 01 01 FF 00 00 46 00 61 00 76 00 6F 00 75 00 72 00 69 00 74 00 65", "61 00 10 46", "61 FE"}
	for i, w := range want {
		if fk.Log[2+3+i] != w {
			t.Errorf("favourite frame %d = %q, want %q", i, fk.Log[2+3+i], w)
		}
	}
	// Empty key writes key type 0 with a single 0 byte and no label.
	if fk.Log[2+9] != "61 FF 00 03 00 FF 00" || fk.Log[2+10] != "61 00 00" {
		t.Errorf("empty key frames: %q %q", fk.Log[2+9], fk.Log[2+10])
	}
	s, err := c.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Layers) != 1 || s.Layers[0].Name != "Rating" {
		t.Fatalf("%+v", s.Layers)
	}
	for i := range ratingKeys {
		if !s.Layers[0].Keys[i].Same(ratingKeys[i]) {
			t.Errorf("key %d reads back as %+v", i, s.Layers[0].Keys[i])
		}
	}
}

func TestWriteTwoLayersThenBackToOne(t *testing.T) {
	fk := vaydeertest.NewFactory()
	c := NewClient(fk)
	two := []Layer{{Name: "Rating", Keys: ratingKeys}, {Name: "Digits", Keys: digitKeys()}}
	if err := c.WriteLayers(two, 6); err != nil {
		t.Fatal(err)
	}
	// Each layer: name with maxLayerIndex 1, 9 keys (3 frames each), commit with maxLayerIndex 1.
	if !strings.HasPrefix(fk.Log[1], "65 00 01 ") || fk.Log[1+1+27] != "66 00 01" ||
		!strings.HasPrefix(fk.Log[1+1+27+1], "65 01 01 ") || fk.Log[len(fk.Log)-1] != "66 01 01" {
		t.Fatalf("frames: %q %q %q %q", fk.Log[1], fk.Log[29], fk.Log[30], fk.Log[len(fk.Log)-1])
	}
	s, err := c.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if s.Info.LayerCount != 2 || len(s.Layers) != 2 || s.Layers[1].Name != "Digits" || !s.Layers[1].Keys[0].Same(digitKeys()[0]) {
		t.Fatalf("%+v", s)
	}
	if err := c.SetActiveLayer(1); err != nil {
		t.Fatal(err)
	}
	if info, _ := c.Info(); info.ActiveLayer != 1 {
		t.Fatalf("active layer %d", info.ActiveLayer)
	}
	if err := c.WriteLayers(two[:1], 6); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.ReadAll(); len(s.Layers) != 1 || s.Info.ActiveLayer != 0 {
		t.Fatalf("after shrinking: %d layers, active %d", len(s.Layers), s.Info.ActiveLayer)
	}
}

func TestLayerCountLimits(t *testing.T) {
	c := NewClient(vaydeertest.NewFactory())
	if err := c.WriteLayers(nil, 6); err == nil {
		t.Fatal("wrote zero layers")
	}
	seven := make([]Layer, 7)
	if err := c.WriteLayers(seven, 6); err == nil {
		t.Fatal("wrote 7 layers")
	}
}

func TestInvalidKeyStopsBeforeWriting(t *testing.T) {
	fk := vaydeertest.NewFactory()
	bad := append([]Key(nil), ratingKeys...)
	bad[4] = Key{Kind: KindOther, RawType: 5}
	if err := NewClient(fk).WriteLayers([]Layer{{Name: "X", Keys: bad}}, 6); err == nil {
		t.Fatal("wrote an unwritable key")
	}
	if len(fk.Log) != 0 {
		t.Fatalf("sent %d frames before refusing", len(fk.Log))
	}
}

func TestRefusedCommandIsAnError(t *testing.T) {
	fk := vaydeertest.NewFactory()
	fk.FailCmd = 0x66
	err := NewClient(fk).WriteLayers([]Layer{{Name: "X", Keys: ratingKeys}}, 6)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("got %v", err)
	}
}

func TestStaleAnswerIsSkipped(t *testing.T) {
	fk := vaydeertest.NewFactory()
	fk.Answer(0x67, 0, 0, 'X') // left over from an earlier conversation
	info, err := NewClient(fk).Info()
	if err != nil || info.Keys != 9 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestKeyValidate(t *testing.T) {
	bad := []Key{
		{Kind: KindKey},
		{Kind: KindCombo, Codes: []byte{16}},
		{Kind: KindKey, Label: "this name is far too long", Codes: []byte{'A'}},
		{Kind: KindOther, RawType: 5},
		{Kind: "macro"},
	}
	for _, k := range bad {
		if k.Validate() == nil {
			t.Errorf("accepted %+v", k)
		}
	}
}

func TestLongestLabelFitsInOneFrame(t *testing.T) {
	head := append([]byte{0xFF, 0, 8, 1, 0xFF, 0}, encodeUTF16BE(strings.Repeat("W", MaxLabel))...)
	if _, err := BuildFrame(0x61, head); err != nil {
		t.Fatal(err)
	}
}

func TestInfoTriesTwiceOnNoAnswer(t *testing.T) {
	fk := vaydeertest.NewFactory()
	fk.MuteOnce = 0x60
	info, err := NewClient(fk).Info()
	if err != nil || info.Keys != 9 {
		t.Fatalf("%+v %v", info, err)
	}
	if fk.MuteOnce != 0 {
		t.Error("the first, unanswered device-info command was never sent")
	}
}
