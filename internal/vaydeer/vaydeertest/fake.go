// Package vaydeertest has a simulated keypad for tests. It answers like the real one
// (answers recorded on firmware 1.1.5 have exactly this shape). Layers follow the vendor
// app's model: a commit with maxLayerIndex m leaves m+1 layers.
package vaydeertest

import (
	"fmt"
	"sync"
	"time"

	"ninekeypad/internal/hid"
)

// Fake is a simulated keypad command interface. It implements hid.Device.
type Fake struct {
	mu      sync.Mutex
	names   []string
	layers  [][9]fakeKey
	active  int
	pending *fakeKey
	pendL   int
	pendK   int
	out     [][]byte

	Log      []string // every command received: "61 FF 00 01 ..."
	Commits  int      // number of commit (66) commands
	FailCmd  byte     // when set, this command is answered with status 1
	MuteOnce byte     // when set, this command gets no answer the first time (then it is cleared)
}

type fakeKey struct {
	typ, sub byte
	label    []byte
	data     []byte
}

// NewFactory is a keypad as it comes from the factory: one layer "Layer1", keys 7 8 9 / 4 5 6 / 1 2 3.
func NewFactory() *Fake {
	var keys [9]fakeKey
	for i, d := range "789456123" {
		keys[i] = fakeKey{typ: 0, sub: 0xFF, label: []byte{0, byte(d)}, data: []byte{byte(d)}}
	}
	return &Fake{names: []string{"Layer1"}, layers: [][9]fakeKey{keys}}
}

// Answer queues an answer, as if the keypad had sent it.
func (k *Fake) Answer(cmd byte, status byte, data ...byte) {
	body := append([]byte{cmd, byte(1 + len(data)), status}, data...)
	var x byte
	for _, b := range body {
		x ^= b
	}
	rep := append(body, x)
	k.out = append(k.out, append(rep, make([]byte, 64-len(rep))...))
}

// ActiveLayer is the layer the simulated keypad uses now.
func (k *Fake) ActiveLayer() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.active
}

func (k *Fake) grow(layer int) {
	for len(k.layers) <= layer {
		k.layers = append(k.layers, [9]fakeKey{})
		k.names = append(k.names, "")
	}
}

func (k *Fake) Write(report []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(report) != 65 || report[0] != 0 {
		return fmt.Errorf("bad report")
	}
	cmd, n := report[1], int(report[2])
	if k.MuteOnce != 0 && cmd == k.MuteOnce {
		k.MuteOnce = 0
		return nil
	}
	p := report[3 : 3+n]
	var x byte
	for _, b := range report[1 : 3+n] {
		x ^= b
	}
	if x != report[3+n] {
		return fmt.Errorf("bad checksum from host")
	}
	k.Log = append(k.Log, fmt.Sprintf("%02X % X", cmd, p))
	if cmd == k.FailCmd {
		k.Answer(cmd, 1)
		return nil
	}
	switch cmd {
	case 0x60:
		k.Answer(cmd, 0, 0x01, 0x09, 1, 1, 5, 0, 2, 2, byte(k.active), byte(len(k.layers)), 6)
	case 0x67:
		if int(p[0]) >= len(k.names) {
			k.Answer(cmd, 1)
			return nil
		}
		k.Answer(cmd, 0, utf16be(k.names[p[0]])...)
	case 0x62:
		if int(p[1]) >= len(k.layers) {
			k.Answer(cmd, 1)
			return nil
		}
		key := k.layers[p[1]][p[2]]
		switch {
		case p[0] == 0xFF:
			k.Answer(cmd, 0, append([]byte{0xFF, key.typ, key.sub, 0}, key.label...)...)
		case p[0] == 0 && len(key.data) > 0:
			k.Answer(cmd, 0, append([]byte{0}, key.data...)...)
		default:
			k.Answer(cmd, 0, 0xFE)
		}
	case 0x61:
		switch {
		case p[0] == 0xFF:
			k.pending = &fakeKey{typ: p[3], sub: p[4], label: append([]byte(nil), p[6:]...)}
			k.pendL, k.pendK = int(p[1]), int(p[2])
		case p[0] == 0xFE:
			k.grow(k.pendL)
			k.layers[k.pendL][k.pendK] = *k.pending
		default:
			k.pending.data = append(k.pending.data, p[1:]...)
		}
		k.Answer(cmd, 0)
	case 0x65:
		k.grow(int(p[0]))
		k.names[p[0]] = fromUTF16BE(p[2:])
		k.Answer(cmd, 0)
	case 0x66:
		k.Commits++
		count := int(p[1]) + 1
		k.grow(count - 1)
		k.layers, k.names = k.layers[:count], k.names[:count]
		if k.active >= count {
			k.active = 0
		}
		k.Answer(cmd, 0)
	case 0x64:
		if int(p[0]) >= len(k.layers) {
			k.Answer(cmd, 1)
			return nil
		}
		k.active = int(p[0])
		k.Answer(cmd, 0)
	case 0xFD:
		k.Answer(cmd, 0)
	default:
		k.Answer(cmd, 1)
	}
	return nil
}

func (k *Fake) Read(time.Duration) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.out) == 0 {
		return nil, hid.ErrTimeout
	}
	r := k.out[0]
	k.out = k.out[1:]
	return r, nil
}

func (k *Fake) Close() error { return nil }

func utf16be(s string) []byte {
	var out []byte
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func fromUTF16BE(b []byte) string {
	var rs []rune
	for i := 0; i+1 < len(b); i += 2 {
		rs = append(rs, rune(b[i])<<8|rune(b[i+1]))
	}
	return string(rs)
}
