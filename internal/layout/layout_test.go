package layout

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ninekeypad/internal/vaydeer"
)

// rating is an example layout for the tests: stars, a Shift combination and empty keys.
func rating() Layout {
	return New("Rating", []vaydeer.Key{
		key("4 stars", '4'), key("Favourite", 16, 'F'), key("Bookmark", 16, 'B'),
		none(), none(), none(),
		key("3 stars", '3'), key("2 stars", '2'), key("1 star", '1'),
	})
}

func TestPresetsAreValid(t *testing.T) {
	for name, l := range Presets() {
		if err := l.Validate(); err != nil {
			t.Errorf("preset %s: %v", name, err)
		}
	}
	if err := rating().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryPreset(t *testing.T) {
	f := Presets()["factory"]
	for i, d := range "789456123" {
		if f.Keys[i].Kind != vaydeer.KindKey || vaydeer.KeyName(f.Keys[i].Codes[0]) != string(d) {
			t.Errorf("key %d (%s) = %+v", i, Position(i, 9), f.Keys[i])
		}
	}
}

func TestSaveLoadList(t *testing.T) {
	s := Store{Root: t.TempDir()}
	if err := s.CheckWritable(); err != nil {
		t.Fatal(err)
	}
	name, err := s.Save("my: layout?", rating())
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(name, ":?") || !strings.HasSuffix(name, ".json") {
		t.Fatalf("unsafe file name %q", name)
	}
	l, err := s.Load(name)
	if err != nil || l.Name != "Rating" || !l.Keys[1].Same(rating().Keys[1]) {
		t.Fatalf("%+v %v", l, err)
	}
	if list := s.List(LayoutsDir); len(list) != 1 || list[0].Name != name || list[0].Layers[0] != "Rating" {
		t.Fatalf("%+v", list)
	}
	// Key codes are stored as readable numbers, not base64.
	raw, _ := os.ReadFile(filepath.Join(s.Root, LayoutsDir, name))
	if !strings.Contains(string(raw), "\"codes\": [\n        16,\n        70\n      ]") {
		t.Fatalf("codes not stored as numbers:\n%s", raw)
	}
	if _, err := s.Load("../" + name); err == nil {
		t.Fatal("loaded a path outside the folder")
	}
	// No temp file is left behind.
	if _, err := os.Stat(filepath.Join(s.Root, LayoutsDir, name+".tmp")); err == nil {
		t.Fatal("temp file left behind")
	}
}

func TestCleanName(t *testing.T) {
	good := map[string]string{"my layout": "my layout.json", "a/b\\c": "a_b_c.json", " x.json ": "x.json", "con-2": "con-2.json"}
	for in, want := range good {
		if got, err := CleanName(in); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "  ", "...", "json", "con", "NUL", "com1", "lpt9.json", "aux.backup"} {
		if got, err := CleanName(bad); err == nil {
			t.Errorf("%q accepted as %q", bad, got)
		}
	}
}

func TestRenameAndDelete(t *testing.T) {
	s := Store{Root: t.TempDir()}
	a, _ := s.Save("a", rating())
	b, _ := s.Save("b", Presets()["factory"])
	if _, err := s.Rename(a, "b"); err == nil {
		t.Fatal("rename replaced an existing layout")
	}
	if same, err := s.Rename(a, "a"); err != nil || same != a {
		t.Fatalf("rename to the same name: %q %v", same, err)
	}
	c, err := s.Rename(a, "rating final")
	if err != nil || c != "rating final.json" {
		t.Fatalf("%q %v", c, err)
	}
	if _, err := s.Load(a); err == nil {
		t.Fatal("old name still there")
	}
	if up, err := s.Rename(c, "Rating Final"); err != nil || up != "Rating Final.json" {
		t.Fatalf("case-only rename: %q %v", up, err)
	}
	if err := s.Delete(b); err != nil {
		t.Fatal(err)
	}
	if list := s.List(LayoutsDir); len(list) != 1 || list[0].Name != "Rating Final.json" {
		t.Fatalf("%+v", list)
	}
	if err := s.Delete("../x.json"); err == nil {
		t.Fatal("deleted outside the folder")
	}
}

// On a case-sensitive disk (Linux), "work.json" and "Work.json" are two files:
// renaming one to the other must not replace it.
func TestCaseRenameKeepsOtherFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows disks are case-insensitive")
	}
	s := Store{Root: t.TempDir()}
	lower, _ := s.Save("work", rating())
	upper, _ := s.Save("Work", Presets()["factory"])
	if _, err := s.Rename(lower, "Work"); err == nil {
		t.Fatal("replaced the other file")
	}
	if l, err := s.Load(upper); err != nil || l.Name != "Layer1" {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestBackups(t *testing.T) {
	s := Store{Root: t.TempDir()}
	r, f := rating(), Presets()["factory"]
	k := Keypad{Layers: []Layer{{Name: r.Name, Keys: r.Keys}, {Name: f.Name, Keys: f.Keys}}, ActiveLayer: 1}
	name, err := s.SaveBackup(k, "before-write")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_before-write.json") {
		t.Fatalf("name %q", name)
	}
	back, err := s.LoadBackup(name)
	if err != nil || back.Note != "before-write" || len(back.Layers) != 2 || back.ActiveLayer != 1 || !back.Layers[1].Keys[0].Same(f.Keys[0]) {
		t.Fatalf("%+v %v", back, err)
	}
	// A second backup in the same second gets its own file.
	second, err := s.SaveBackup(k, "before-write")
	if err != nil || second == name {
		t.Fatalf("second backup %q %v", second, err)
	}
	list := s.List(BackupsDir)
	if len(list) != 2 || strings.Join(list[0].Layers, ",") != "Rating,Layer1" {
		t.Fatalf("%+v", list)
	}
}

// Backups made by version 0.1.0 hold one layer in the layout format.
func TestOldOneLayerBackupStillLoads(t *testing.T) {
	s := Store{Root: t.TempDir()}
	old := `{"format": "vaydeer-9key-layout", "version": 1, "name": "Layer1", "note": "before-write",
	  "savedAt": "2026-09-25T11:12:14+02:00", "device": {"firmware": "1.1.5", "bootloader": "0.2.2", "layerCount": 1},
	  "keys": [{"kind":"key","label":"7","codes":[55]},{"kind":"none","label":""},{"kind":"none","label":""},
	           {"kind":"none","label":""},{"kind":"none","label":""},{"kind":"none","label":""},
	           {"kind":"none","label":""},{"kind":"none","label":""},{"kind":"none","label":""}]}`
	os.MkdirAll(filepath.Join(s.Root, BackupsDir), 0o755)
	os.WriteFile(filepath.Join(s.Root, BackupsDir, "old.json"), []byte(old), 0o644)
	k, err := s.LoadBackup("old.json")
	if err != nil || len(k.Layers) != 1 || k.Layers[0].Name != "Layer1" || k.Layers[0].Keys[0].Codes[0] != 55 {
		t.Fatalf("%+v %v", k, err)
	}
	if list := s.List(BackupsDir); len(list) != 1 || list[0].Layers[0] != "Layer1" {
		t.Fatalf("%+v", list)
	}
}

func TestKeypadValidate(t *testing.T) {
	r := rating()
	one := Layer{Name: r.Name, Keys: r.Keys}
	if err := (Keypad{Layers: []Layer{one}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (Keypad{}).Validate() == nil {
		t.Fatal("accepted zero layers")
	}
	if (Keypad{Layers: []Layer{one, one, one, one, one, one, one}}).Validate() == nil {
		t.Fatal("accepted 7 layers")
	}
}
