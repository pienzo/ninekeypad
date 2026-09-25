package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ninekeypad/internal/layout"
	"ninekeypad/internal/vaydeer"
	"ninekeypad/internal/vaydeer/vaydeertest"
)

// testServer is a server whose keypad is the simulated one.
func testServer(t *testing.T) (*Server, *vaydeertest.Fake, http.Handler) {
	t.Helper()
	fake := vaydeertest.NewFactory()
	s := New(layout.Store{Root: t.TempDir()}, NewToken())
	s.addr = "127.0.0.1:47821"
	s.open = func() (*vaydeer.Client, error) { return vaydeer.NewClient(fake), nil }
	return s, fake, s.routes()
}

func call(t *testing.T, h http.Handler, s *Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = s.addr
	req.Header.Set("X-Keypad-Token", s.token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestReadSavesFirstBackup(t *testing.T) {
	s, _, h := testServer(t)
	code, out := call(t, h, s, "POST", "/api/read", map[string]any{})
	if code != 200 || out["backup"] == nil {
		t.Fatalf("%d %v", code, out)
	}
	if list := s.Store.List(layout.BackupsDir); len(list) != 1 {
		t.Fatalf("backups: %+v", list)
	}
	// The second read makes no further backup.
	call(t, h, s, "POST", "/api/read", map[string]any{})
	if list := s.Store.List(layout.BackupsDir); len(list) != 1 {
		t.Fatalf("backups after second read: %+v", list)
	}
}

func TestWriteBacksUpWritesAndChecks(t *testing.T) {
	s, fake, h := testServer(t)
	f := layout.Presets()["factory"]
	two := append([]vaydeer.Key(nil), f.Keys...)
	next, _ := vaydeer.LayerKey("next")
	two[4] = next
	want := layout.Keypad{Layers: []layout.Layer{{Name: "One", Keys: f.Keys}, {Name: "Two", Keys: two}}}
	code, out := call(t, h, s, "POST", "/api/write", want)
	if code != 200 || out["verified"] != true || out["backup"] == nil {
		t.Fatalf("%d %v", code, out)
	}
	if fake.Commits != 2 {
		t.Fatalf("commits %d", fake.Commits)
	}
	// The layer-switch key is known right after the write.
	if to, _, ok := s.layerSwitch(vaydeer.KeyEvent{Layer: 1, Key: 4, Down: true}); !ok || to != 0 {
		t.Fatalf("layer key not learned: %d %v", to, ok)
	}
	code, _ = call(t, h, s, "POST", "/api/active", map[string]int{"layer": 1})
	if code != 200 || fake.ActiveLayer() != 1 {
		t.Fatalf("set active: %d, layer %d", code, fake.ActiveLayer())
	}
	s.onKey(vaydeer.KeyEvent{Layer: 1, Key: 0, Down: true}) // "7": nothing happens
	if fake.ActiveLayer() != 1 {
		t.Fatal("a normal key switched the layer")
	}
	s.onKey(vaydeer.KeyEvent{Layer: 1, Key: 4, Down: true}) // "next layer" on layer 2 -> layer 1
	if fake.ActiveLayer() != 0 {
		t.Fatalf("layer key: active layer %d", fake.ActiveLayer())
	}
	if code, _ := call(t, h, s, "POST", "/api/active", map[string]int{"layer": 4}); code != 400 {
		t.Fatalf("set active to a missing layer: %d", code)
	}
}

func TestWriteRefusesWrongKeyCount(t *testing.T) {
	s, fake, h := testServer(t)
	four := make([]vaydeer.Key, 4)
	for i := range four {
		four[i] = vaydeer.Key{Kind: vaydeer.KindNone}
	}
	code, out := call(t, h, s, "POST", "/api/write", layout.Keypad{Layers: []layout.Layer{{Name: "Four", Keys: four}}})
	if code != 400 || !strings.Contains(out["error"].(string), "4-key") {
		t.Fatalf("%d %v", code, out)
	}
	for _, l := range fake.Log {
		if strings.HasPrefix(l, "61") || strings.HasPrefix(l, "65") || strings.HasPrefix(l, "FD") {
			t.Fatalf("wrote to the keypad: %q", l)
		}
	}
}

func TestAccessChecks(t *testing.T) {
	s, _, h := testServer(t)
	do := func(host, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Host = host
		if token != "" {
			req.Header.Set("X-Keypad-Token", token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(s.addr, s.token); rec.Code != 200 {
		t.Fatalf("good request: %d", rec.Code)
	}
	if rec := do("localhost:47821", s.token); rec.Code != 200 {
		t.Fatalf("localhost: %d", rec.Code)
	}
	for _, c := range []struct{ host, token string }{
		{s.addr, ""}, {s.addr, NewToken()}, {"evil.example:47821", s.token}, {"127.0.0.1:1", s.token},
	} {
		rec := do(c.host, c.token)
		if rec.Code != 403 {
			t.Errorf("%+v: %d", c, rec.Code)
		}
		// A wrong key gets a readable reason, not "Program not running".
		if !strings.Contains(rec.Body.String(), "old or missing key") {
			t.Errorf("%+v: body %q", c, rec.Body.String())
		}
	}
	// hello shows only the fingerprint, never the token.
	req := httptest.NewRequest("GET", "/api/hello", nil)
	req.Host = s.addr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), s.token) || !strings.Contains(rec.Body.String(), tokenID(s.token)) {
		t.Fatalf("hello: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %v", rec.Header())
	}
}
