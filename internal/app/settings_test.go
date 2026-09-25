package app

import (
	"os"
	"path/filepath"
	"testing"

	"ninekeypad/internal/layout"
)

func TestSettingsStayTheSame(t *testing.T) {
	root := t.TempDir()
	s, changed := LoadSettings(root)
	if !changed || s.Port != DefaultPort || !tokenRe.MatchString(s.Token) {
		t.Fatalf("fresh settings: %+v changed=%v", s, changed)
	}
	if err := SaveSettings(root, s); err != nil {
		t.Fatal(err)
	}
	again, changed := LoadSettings(root)
	if changed || again != s {
		t.Fatalf("reloaded %+v changed=%v, want %+v", again, changed, s)
	}
}

// Settings saved before the rename to ninekeypad keep the bookmarked address.
func TestOldSettingsFileIsUsed(t *testing.T) {
	root := t.TempDir()
	old := `{"port": 47821, "token": "0123456789abcdef0123456789abcdef"}`
	os.WriteFile(filepath.Join(root, oldSettingsFile), []byte(old), 0o644)
	s, changed := LoadSettings(root)
	if !changed || s.Token != "0123456789abcdef0123456789abcdef" || s.Port != 47821 {
		t.Fatalf("%+v changed=%v", s, changed)
	}
}

func TestBrokenSettingsAreReplaced(t *testing.T) {
	root := t.TempDir()
	for _, content := range []string{"not json", `{"port": 0, "token": "abc"}`, `{"port": 47821, "token": "short"}`} {
		os.WriteFile(filepath.Join(root, SettingsFile), []byte(content), 0o644)
		if s, changed := LoadSettings(root); !changed || !tokenRe.MatchString(s.Token) {
			t.Errorf("%q gave %+v changed=%v", content, s, changed)
		}
	}
}

func TestAlreadyRunning(t *testing.T) {
	token := NewToken()
	srv := New(layout.Store{Root: t.TempDir()}, token)
	ln, port, err := srv.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.stop()
	if !AlreadyRunning(port, token) {
		t.Fatal("running server not recognised")
	}
	if AlreadyRunning(port, NewToken()) {
		t.Fatal("wrong token accepted")
	}
	if _, _, err := New(layout.Store{}, token).Listen(port); err == nil {
		t.Fatal("second server got the same port")
	}
}
