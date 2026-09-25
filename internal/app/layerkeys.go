package app

import (
	"fmt"
	"log"

	"ninekeypad/internal/layout"
	"ninekeypad/internal/vaydeer"
)

// The keypad cannot switch layers by itself. While this program runs, it watches key
// presses and switches the layer when a layer-switch key is pressed (see vaydeer/layerkey.go).

// learn remembers what the keypad holds, so key presses can be looked up.
func (s *Server) learn(k layout.Keypad) {
	s.keysMu.Lock()
	s.layers = append([]layout.Layer(nil), k.Layers...)
	s.keysMu.Unlock()
}

// relearn reads the keypad; called when the keypad (re)appears.
func (s *Server) relearn() {
	var snap vaydeer.Snapshot
	err := s.withKeypad(func(c *vaydeer.Client) (err error) { snap, err = c.ReadAll(); return })
	if err != nil {
		log.Printf("Could not read the keypad's layers (%s); layer-switch keys are off until it is read.", friendly(err))
		return
	}
	s.learn(layout.FromSnapshot(snap))
}

// layerSwitch is the layer a key press leads to, if the pressed key is a layer-switch key.
func (s *Server) layerSwitch(ev vaydeer.KeyEvent) (int, string, bool) {
	s.keysMu.Lock()
	defer s.keysMu.Unlock()
	if !ev.Down || ev.Layer < 0 || ev.Layer >= len(s.layers) {
		return 0, "", false
	}
	keys := s.layers[ev.Layer].Keys
	if ev.Key < 0 || ev.Key >= len(keys) || keys[ev.Key].Kind != vaydeer.KindLayer {
		return 0, "", false
	}
	to, ok := vaydeer.SwitchTarget(keys[ev.Key].Layer, ev.Layer, len(s.layers))
	if !ok {
		return 0, "", false
	}
	return to, s.layers[to].Name, true
}

func (s *Server) onKey(ev vaydeer.KeyEvent) {
	if _, _, ok := s.layerSwitch(ev); !ok {
		return // most presses: nothing to do, and no need to wait for the keypad
	}
	var to int
	var name string
	switched := false
	err := s.withKeypad(func(c *vaydeer.Client) error {
		// Decide again now that we have the keypad: a write may have changed the layers
		// while we waited.
		var ok bool
		if to, name, ok = s.layerSwitch(ev); !ok {
			return nil
		}
		switched = true
		return c.SetActiveLayer(to)
	})
	if err != nil {
		log.Printf("Could not switch to layer %d: %s", to+1, friendly(err))
		return
	}
	if switched {
		log.Printf("Switched to layer %d %q.", to+1, name)
		s.events.publish(fmt.Sprintf(`{"layer":%d,"key":-1,"down":false}`, to))
	}
}
