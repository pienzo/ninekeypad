// Package app is the local web server behind the keypad page. It listens on 127.0.0.1
// only and every API call must carry the random token printed in the page address.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"ninekeypad/internal/hid"
	"ninekeypad/internal/layout"
	"ninekeypad/internal/vaydeer"
)

//go:embed web
var webFiles embed.FS

// Version is shown on the page.
const Version = "0.2.0"

// linuxTested says whether this build was tried with a real keypad on Linux; the page shows
// a warning while it is false.
const linuxTested = true

type Server struct {
	Store   layout.Store
	token   string
	addr    string
	devMu   sync.Mutex // one keypad conversation at a time
	events  *eventHub
	quit    chan struct{}
	quitOne sync.Once

	// open opens the keypad's command interface; tests put a simulated keypad here.
	open func() (*vaydeer.Client, error)

	keysMu sync.Mutex
	layers []layout.Layer // what the keypad holds, for the layer-switch keys
}

// New makes a server that accepts only callers who know token.
func New(store layout.Store, token string) *Server {
	s := &Server{Store: store, token: token, events: newEventHub(), quit: make(chan struct{}), open: vaydeer.Open}
	s.events.onConnect = s.relearn
	s.events.onKey = s.onKey
	return s
}

// NewToken makes a random secret for the page address.
func NewToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// PageURL is the page address for a port and token.
func PageURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, token)
}

// tokenID is a fingerprint of the token. It tells whether a running program has our
// token without ever sending the token itself.
func tokenID(token string) string {
	sum := sha256.Sum256([]byte("ninekeypad-id:" + token))
	return hex.EncodeToString(sum[:])
}

// Listen binds 127.0.0.1 on the given port (0 = any free port) and returns the port it got.
func (s *Server) Listen(port int) (net.Listener, int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, 0, err
	}
	s.addr = ln.Addr().String()
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

// AlreadyRunning reports whether this program, with this token, already answers on port.
// It sends nothing secret: it only compares the fingerprint the other side shows.
func AlreadyRunning(port int, token string) bool {
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/api/hello", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var hello struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<10)).Decode(&hello) != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hello.ID), []byte(tokenID(token))) == 1
}

// Serve runs until Quit is pressed. It keeps running when the page is closed, because it also
// works the layer-switch keys.
func (s *Server) Serve(ln net.Listener) error {
	go s.events.run()
	srv := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-s.quit
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) stop() { s.quitOne.Do(func() { close(s.quit) }) }

// Busy reports whether the program is talking to the keypad right now.
func (s *Server) Busy() bool {
	if s.devMu.TryLock() {
		s.devMu.Unlock()
		return false
	}
	return true
}

// WaitIdle waits until a running keypad conversation (e.g. a write) has finished, then keeps
// the keypad closed to further commands, so the program can exit without leaving a
// half-written keypad. It gives up after limit.
func (s *Server) WaitIdle(limit time.Duration) bool {
	done := make(chan struct{})
	go func() {
		s.devMu.Lock() // never unlocked: the program is about to exit
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer") // the token is in the page address
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// decodeJSON reads a small JSON request body.
func decodeJSON(r *http.Request, limit int64, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(v); err != nil {
		return fmt.Errorf("bad request: %w", err)
	}
	return nil
}

const wrongKeyMessage = "This page address has an old or missing key. Use the address shown in the keypad program's window."

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFiles, "web")
	files := http.FileServerFS(static)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		files.ServeHTTP(w, r)
	})

	api := func(pattern string, h func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			securityHeaders(w)
			if !s.authorized(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": wrongKeyMessage, "code": "bad-key"})
				return
			}
			v, err := h(r)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, v)
		})
	}
	api("GET /api/status", s.status)
	api("GET /api/keycodes", func(*http.Request) (any, error) { return vaydeer.KeyCodes, nil })
	api("GET /api/presets", func(*http.Request) (any, error) { return layout.Presets(), nil })
	api("POST /api/read", s.read)
	api("POST /api/write", s.write)
	api("GET /api/files", s.listFiles)
	api("GET /api/file", s.loadFile)
	api("POST /api/file", s.saveFile)
	api("POST /api/file/rename", s.renameFile)
	api("POST /api/file/delete", s.deleteFile)
	api("POST /api/active", s.setActive)
	api("POST /api/quit", func(*http.Request) (any, error) {
		time.AfterFunc(300*time.Millisecond, s.stop)
		return "ok", nil
	})
	// Needs no token: AlreadyRunning asks it. It shows only the token's fingerprint.
	mux.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if !s.hostOK(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"app": "ninekeypad", "id": tokenID(s.token)})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if !s.authorized(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": wrongKeyMessage, "code": "bad-key"})
			return
		}
		s.events.serve(w, r)
	})
	return mux
}

// hostOK blocks DNS rebinding: a web site whose name points to 127.0.0.1 still sends its
// own name in the Host header.
func (s *Server) hostOK(r *http.Request) bool {
	_, port, _ := net.SplitHostPort(s.addr)
	return r.Host == s.addr || r.Host == "localhost:"+port
}

// authorized checks the Host header and the token (header, or ?t= for EventSource).
func (s *Server) authorized(r *http.Request) bool {
	if !s.hostOK(r) {
		return false
	}
	t := r.Header.Get("X-Keypad-Token")
	if t == "" {
		t = r.URL.Query().Get("t")
	}
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

type statusReply struct {
	Version     string        `json:"version"`
	Platform    string        `json:"platform"`
	LinuxTested bool          `json:"linuxTested"`
	Connected   bool          `json:"connected"`
	Error       string        `json:"error,omitempty"`
	Firmware    string        `json:"firmware,omitempty"`
	Bootloader  string        `json:"bootloader,omitempty"`
	Info        *vaydeer.Info `json:"info,omitempty"`
	Folder      string        `json:"folder"`
	FolderOK    bool          `json:"folderOK"`
	FolderError string        `json:"folderError,omitempty"`
}

// status reports whether the keypad answers. With ?folder=1 it also proves the program
// folder takes files (the page polls without it, to avoid a disk write every few seconds).
func (s *Server) status(r *http.Request) (any, error) {
	rep := statusReply{Version: Version, Platform: runtime.GOOS, LinuxTested: linuxTested, Folder: s.Store.Root, FolderOK: true}
	if r.URL.Query().Get("folder") == "1" {
		if err := s.Store.CheckWritable(); err != nil {
			rep.FolderOK, rep.FolderError = false, err.Error()
		}
	}
	err := s.withKeypad(func(c *vaydeer.Client) error {
		info, err := c.Info()
		if err != nil {
			return err
		}
		rep.Info, rep.Firmware, rep.Bootloader = &info, info.FirmwareString(), info.BootloaderString()
		return nil
	})
	if err != nil {
		rep.Error = friendly(err)
	} else {
		rep.Connected = true
	}
	return rep, nil
}

// withKeypad opens the keypad exclusively for one conversation. If another program holds
// it for a moment (e.g. a second copy of this program polling), it retries briefly.
func (s *Server) withKeypad(f func(*vaydeer.Client) error) error {
	s.devMu.Lock()
	defer s.devMu.Unlock()
	var c *vaydeer.Client
	var err error
	for try := 0; try < 10; try++ {
		if c, err = s.open(); !errors.Is(err, hid.ErrBusy) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	defer c.Close()
	return f(c)
}

func friendly(err error) string {
	switch {
	case errors.Is(err, hid.ErrPermission) && runtime.GOOS == "linux":
		return "No permission to use the keypad. Close this program and start it again with: sudo ./ninekeypad"
	case errors.Is(err, hid.ErrPermission), errors.Is(err, hid.ErrBusy):
		return "The keypad is in use by another program (another copy of this program, or the Vaydeer app). Close it and try again."
	}
	return err.Error()
}

// otherKeys lists keys this program can show but not write (text, macro, mouse, program).
// A backup holding such keys cannot be fully restored by this program.
func otherKeys(k layout.Keypad) []string {
	var out []string
	for li, l := range k.Layers {
		for ki, key := range l.Keys {
			if key.Kind == vaydeer.KindOther {
				out = append(out, fmt.Sprintf("layer %d key %d", li+1, ki+1))
			}
		}
	}
	return out
}

type readReply struct {
	Keypad   layout.Keypad `json:"keypad"`
	Backup   string        `json:"backup,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
}

// read gets every layer on the keypad. The first read ever also saves a backup of it.
func (s *Server) read(*http.Request) (any, error) {
	var snap vaydeer.Snapshot
	if err := s.withKeypad(func(c *vaydeer.Client) (err error) { snap, err = c.ReadAll(); return }); err != nil {
		return nil, errors.New(friendly(err))
	}
	rep := readReply{Keypad: layout.FromSnapshot(snap)}
	s.learn(rep.Keypad)
	if len(s.Store.List(layout.BackupsDir)) == 0 {
		name, err := s.Store.SaveBackup(rep.Keypad, "first-read")
		if err != nil {
			// Still show the keypad; the page already warns in red that the folder takes no files.
			rep.Warnings = append(rep.Warnings, "Could not save a first backup: "+err.Error())
		} else {
			rep.Backup = name
		}
	}
	if o := otherKeys(rep.Keypad); len(o) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%d key(s) hold text, a macro, a mouse action or a program (%v). This program shows them but cannot write them back: "+
				"if you change this keypad here, a backup cannot restore those keys; only the Vaydeer app can.", len(o), o))
	}
	return rep, nil
}

type writeReply struct {
	Backup   string        `json:"backup"`
	Verified bool          `json:"verified"`
	Problems []string      `json:"problems,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
	Keypad   layout.Keypad `json:"keypad"`
}

// write: read + back up everything on the keypad, write all layers, read them back and compare.
func (s *Server) write(r *http.Request) (any, error) {
	var want layout.Keypad
	if err := decodeJSON(r, 1<<20, &want); err != nil {
		return nil, err
	}
	if err := want.Validate(); err != nil {
		return nil, err
	}
	var rep writeReply
	err := s.withKeypad(func(c *vaydeer.Client) error {
		before, err := c.ReadAll()
		if err != nil {
			return fmt.Errorf("reading the keypad before writing: %w", err)
		}
		if want.KeyCount() != before.Info.Keys {
			return fmt.Errorf("these layers are for a %d-key keypad, but the connected keypad has %d keys", want.KeyCount(), before.Info.Keys)
		}
		if len(want.Layers) > before.Info.MaxLayers {
			return fmt.Errorf("this keypad holds at most %d layers", before.Info.MaxLayers)
		}
		old := layout.FromSnapshot(before)
		if rep.Backup, err = s.Store.SaveBackup(old, "before-write"); err != nil {
			return fmt.Errorf("nothing was written, because the backup could not be saved: %w", err)
		}
		restoreNote := fmt.Sprintf("load backup %s and write it to restore", rep.Backup)
		if o := otherKeys(old); len(o) > 0 {
			restoreNote += fmt.Sprintf(" (except %v, which only the Vaydeer app can write)", o)
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("Backup %s holds key(s) this program cannot write back: %v.", rep.Backup, o))
		}
		if err := c.WriteLayers(want.VaydeerLayers(), before.Info.MaxLayers); err != nil {
			return fmt.Errorf("writing stopped part-way: %w. The keypad may be half-written; %s", err, restoreNote)
		}
		after, err := c.ReadAll()
		if err != nil {
			return fmt.Errorf("written, but reading it back failed: %w", err)
		}
		// Keep the layer that was active, if it still exists.
		if after.Info.ActiveLayer >= len(after.Layers) {
			if c.SetActiveLayer(0) == nil {
				after.Info.ActiveLayer = 0
			}
		}
		rep.Keypad = layout.FromSnapshot(after)
		s.learn(rep.Keypad)
		rep.Problems = compareLayers(want.Layers, rep.Keypad.Layers)
		rep.Verified = len(rep.Problems) == 0
		return nil
	})
	if err != nil {
		return nil, errors.New(friendly(err))
	}
	return rep, nil
}

func compareLayers(want, got []layout.Layer) []string {
	var p []string
	if len(got) != len(want) {
		p = append(p, fmt.Sprintf("the keypad reports %d layers, not %d", len(got), len(want)))
	}
	for i := 0; i < len(want) && i < len(got); i++ {
		if got[i].Name != want[i].Name {
			p = append(p, fmt.Sprintf("layer %d name reads back as %q, not %q", i+1, got[i].Name, want[i].Name))
		}
		if len(got[i].Keys) != len(want[i].Keys) {
			p = append(p, fmt.Sprintf("layer %d reads back with %d keys, not %d", i+1, len(got[i].Keys), len(want[i].Keys)))
			continue
		}
		for k := range want[i].Keys {
			if !got[i].Keys[k].Same(want[i].Keys[k]) {
				p = append(p, fmt.Sprintf("layer %d, key %d (%s) reads back differently", i+1, k+1, layout.Position(k, len(want[i].Keys))))
			}
		}
	}
	return p
}

// setActive switches the keypad to another of its stored layers.
func (s *Server) setActive(r *http.Request) (any, error) {
	var req struct {
		Layer int `json:"layer"`
	}
	if err := decodeJSON(r, 1<<10, &req); err != nil {
		return nil, err
	}
	err := s.withKeypad(func(c *vaydeer.Client) error {
		info, err := c.Info()
		if err != nil {
			return err
		}
		if req.Layer < 0 || req.Layer >= info.LayerCount {
			return fmt.Errorf("the keypad has no layer %d", req.Layer+1)
		}
		return c.SetActiveLayer(req.Layer)
	})
	if err != nil {
		return nil, errors.New(friendly(err))
	}
	return "ok", nil
}

func dirParam(r *http.Request) (string, error) {
	switch d := r.URL.Query().Get("dir"); d {
	case layout.LayoutsDir, layout.BackupsDir:
		return d, nil
	default:
		return "", errors.New("unknown folder")
	}
}

func (s *Server) listFiles(r *http.Request) (any, error) {
	dir, err := dirParam(r)
	if err != nil {
		return nil, err
	}
	return s.Store.List(dir), nil
}

// loadFile returns a layout (layouts/) or a whole keypad (backups/).
func (s *Server) loadFile(r *http.Request) (any, error) {
	dir, err := dirParam(r)
	if err != nil {
		return nil, err
	}
	name := r.URL.Query().Get("name")
	if dir == layout.BackupsDir {
		return s.Store.LoadBackup(name)
	}
	return s.Store.Load(name)
}

func (s *Server) saveFile(r *http.Request) (any, error) {
	var req struct {
		Name   string        `json:"name"`
		Layout layout.Layout `json:"layout"`
	}
	if err := decodeJSON(r, 1<<20, &req); err != nil {
		return nil, err
	}
	req.Layout.Format = layout.Format
	if err := req.Layout.Validate(); err != nil {
		return nil, err
	}
	name, err := s.Store.Save(req.Name, req.Layout)
	if err != nil {
		return nil, fmt.Errorf("could not save: %w", err)
	}
	return map[string]string{"name": name}, nil
}

// Only layouts can be renamed or deleted from the page; backups stay.
func (s *Server) renameFile(r *http.Request) (any, error) {
	var req struct {
		Name    string `json:"name"`
		NewName string `json:"newName"`
	}
	if err := decodeJSON(r, 1<<10, &req); err != nil {
		return nil, err
	}
	name, err := s.Store.Rename(req.Name, req.NewName)
	if err != nil {
		return nil, fmt.Errorf("could not rename: %w", err)
	}
	return map[string]string{"name": name}, nil
}

func (s *Server) deleteFile(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, 1<<10, &req); err != nil {
		return nil, err
	}
	if err := s.Store.Delete(req.Name); err != nil {
		return nil, fmt.Errorf("could not delete: %w", err)
	}
	return "ok", nil
}
