package vaydeer

// KeyEvent is one physical key press or release, from the key-event interface.
type KeyEvent struct {
	Layer int
	Key   int // 0..8, left to right, top to bottom
	Down  bool
}

// ParseKeyEvent decodes [0xFB, 0x03, layer, key, state, xor, ...]; state 0 = pressed, 2 = released.
func ParseKeyEvent(rep []byte) (KeyEvent, bool) {
	if len(rep) < 6 || rep[0] != 0xFB || rep[1] != 0x03 {
		return KeyEvent{}, false
	}
	if rep[0]^rep[1]^rep[2]^rep[3]^rep[4] != rep[5] || rep[3] > 8 {
		return KeyEvent{}, false
	}
	return KeyEvent{Layer: int(rep[2]), Key: int(rep[3]), Down: rep[4] == 0}, true
}
