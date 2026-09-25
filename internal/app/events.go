package app

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"ninekeypad/internal/hid"
	"ninekeypad/internal/vaydeer"
)

// eventHub reads the keypad's key-event interface (interface 2) and passes every key
// press to the open pages, so the grid lights up the key you press.
// On Linux, reading this interface is also what keeps the keypad's keys working.
type eventHub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}

	onConnect func()                 // the keypad appeared
	onKey     func(vaydeer.KeyEvent) // a key was pressed or released
}

func newEventHub() *eventHub { return &eventHub{subs: map[chan string]struct{}{}} }

func (h *eventHub) publish(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default: // a slow page misses an event rather than blocking the keypad
		}
	}
}

func (h *eventHub) run() {
	WatchKeys(h.onConnect, func(ev vaydeer.KeyEvent) {
		h.publish(fmt.Sprintf(`{"layer":%d,"key":%d,"down":%t}`, ev.Layer, ev.Key, ev.Down))
		if h.onKey != nil {
			h.onKey(ev)
		}
	}, nil)
}

// WatchKeys listens to the keypad's key-event interface forever, read-only, and reconnects
// when the keypad is unplugged and plugged in again. onConnect and onErr may be nil;
// onErr hears why the keypad could not be opened (e.g. no permission on Linux).
func WatchKeys(onConnect func(), onKey func(vaydeer.KeyEvent), onErr func(error)) {
	for {
		err := watchOnce(onConnect, onKey)
		if err != nil && onErr != nil {
			onErr(err)
		}
		time.Sleep(2 * time.Second) // keypad unplugged or not yet there: try again
	}
}

func watchOnce(onConnect func(), onKey func(vaydeer.KeyEvent)) error {
	path, err := vaydeer.FindInterface(vaydeer.EventUsage)
	if err != nil {
		return err
	}
	dev, err := hid.OpenReader(path)
	if err != nil {
		return err
	}
	defer dev.Close()
	if onConnect != nil {
		onConnect()
	}
	for {
		rep, err := dev.Read(time.Minute)
		if err == hid.ErrTimeout {
			continue
		}
		if err != nil {
			return nil // unplugged: WatchKeys tries again
		}
		if ev, ok := vaydeer.ParseKeyEvent(rep); ok {
			onKey(ev)
		}
	}
}

func (h *eventHub) serve(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := make(chan string, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}()
	fmt.Fprint(w, ": hello\n\n")
	fl.Flush()
	keep := time.NewTicker(25 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-keep.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			fl.Flush()
		}
	}
}
