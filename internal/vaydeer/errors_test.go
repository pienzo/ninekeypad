package vaydeer

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ninekeypad/internal/hid"
)

func TestParseResponseRejectsBrokenAnswers(t *testing.T) {
	cases := map[string][]byte{
		"too short":         {0x60, 0x01, 0x00},
		"zero length":       {0x60, 0x00, 0x00, 0x60},
		"length past end":   {0x60, 0x09, 0x00, 0x01, 0x02},
		"checksum mismatch": {0x60, 0x01, 0x00, 0x00},
	}
	for name, resp := range cases {
		if _, _, err := ParseResponse(0x60, resp); err == nil || err == errWrongCommand {
			t.Errorf("%s: got %v", name, err)
		}
	}
	// An answer to another command is recognised as stale even with a bad checksum.
	if _, _, err := ParseResponse(0x60, []byte{0x67, 0x01, 0x00, 0xAA}); err != errWrongCommand {
		t.Errorf("stale answer: got %v", err)
	}
}

// scripted is a device that answers from a fixed list and then times out.
type scripted struct{ answers [][]byte }

func (s *scripted) Write([]byte) error { return nil }
func (s *scripted) Close() error       { return nil }
func (s *scripted) Read(time.Duration) ([]byte, error) {
	if len(s.answers) == 0 {
		return nil, hid.ErrTimeout
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return a, nil
}

func answer(cmd, status byte, data ...byte) []byte {
	body := append([]byte{cmd, byte(1 + len(data)), status}, data...)
	var x byte
	for _, b := range body {
		x ^= b
	}
	return append(body, x)
}

func TestDoTimesOut(t *testing.T) {
	_, err := NewClient(&scripted{}).Do(0x60)
	if !errors.Is(err, hid.ErrTimeout) {
		t.Fatalf("got %v", err)
	}
}

func TestDoGivesUpOnEndlessStaleAnswers(t *testing.T) {
	var stale [][]byte
	for i := 0; i < 20; i++ {
		stale = append(stale, answer(0x67, 0))
	}
	_, err := NewClient(&scripted{answers: stale}).Do(0x60)
	if err == nil || !strings.Contains(err.Error(), "no matching answer") {
		t.Fatalf("got %v", err)
	}
}

func TestReadKeyThatNeverEnds(t *testing.T) {
	answers := [][]byte{answer(0x62, 0, 0xFF, 0, 0xFF, 0)}
	for seq := 0; seq < 16; seq++ {
		answers = append(answers, answer(0x62, 0, byte(seq), 'A'))
	}
	_, err := NewClient(&scripted{answers: answers}).ReadKey(0, 0)
	if err == nil || !strings.Contains(err.Error(), "did not end") {
		t.Fatalf("got %v", err)
	}
}
