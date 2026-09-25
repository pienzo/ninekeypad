package layout

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Store keeps layout files in <program folder>/layouts and backups in <program folder>/backups.
type Store struct {
	Root string
}

const (
	LayoutsDir = "layouts"
	BackupsDir = "backups"
)

var safeName = regexp.MustCompile(`[^A-Za-z0-9 _.-]+`)

// Windows maps these names to devices, whatever the extension ("con.json" is the console).
var reservedName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)

// CleanName turns what the user typed into a safe file name ending in .json.
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(safeName.ReplaceAllString(strings.TrimSpace(name), "_"))
	name = strings.Trim(name, ".")
	if name == "" || strings.EqualFold(name, "json") {
		return "", errors.New("please give a file name")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	if reservedName.MatchString(name) {
		return "", fmt.Errorf("%q is a name Windows keeps for devices; please choose another", strings.TrimSuffix(name, ".json"))
	}
	return name, nil
}
func checkName(name string) error {
	if name == "" || name != filepath.Base(name) || !strings.HasSuffix(strings.ToLower(name), ".json") {
		return errors.New("bad file name")
	}
	return nil
}

// Save writes a layout into layouts/ and returns the file name.
func (s Store) Save(name string, l Layout) (string, error) {
	name, err := CleanName(name)
	if err != nil {
		return "", err
	}
	l.Format, l.Version, l.Note = Format, 1, ""
	l.SavedAt = time.Now().Format(time.RFC3339)
	if err := s.writeJSON(LayoutsDir, name, l); err != nil {
		return "", err
	}
	if _, err := s.Load(name); err != nil {
		return "", fmt.Errorf("file was written but cannot be read back: %w", err)
	}
	return name, nil
}

// SaveBackup stores a timestamped backup of a whole keypad and returns its file name.
func (s Store) SaveBackup(k Keypad, why string) (string, error) {
	k.Format, k.Version, k.Note = FormatKeypad, 1, why
	k.SavedAt = time.Now().Format(time.RFC3339)
	stamp := time.Now().Format("2006-01-02_15-04-05")
	name := stamp + "_" + why + ".json"
	for i := 2; s.exists(BackupsDir, name); i++ { // never replace an earlier backup
		name = fmt.Sprintf("%s_%s-%d.json", stamp, why, i)
	}
	if err := s.writeJSON(BackupsDir, name, k); err != nil {
		return "", err
	}
	// Written and flushed to the disk (WriteFile syncs); now prove it reads back.
	if _, err := s.LoadBackup(name); err != nil {
		return "", fmt.Errorf("backup was written but cannot be read back: %w", err)
	}
	return name, nil
}

func (s Store) exists(dir, name string) bool {
	_, err := os.Lstat(filepath.Join(s.Root, dir, name))
	return err == nil
}

func (s Store) writeJSON(dir, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0o644)
}

// WriteFile writes rel (relative to Root) completely or not at all: a temp file, flushed to
// the disk, then renamed over the old one. Under sudo on Linux, only what it creates gets
// handed to the desktop user; folders that already existed keep their owner.
func (s Store) WriteFile(rel string, data []byte, perm os.FileMode) error {
	final := filepath.Join(s.Root, rel)
	folder := filepath.Dir(final)
	if err := s.makeFolder(folder); err != nil {
		return err
	}
	tmp := final + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	syncFolder(folder)
	fixOwner(final)
	return nil
}

// makeFolder creates folder (and parents) and hands over only the ones it created.
func (s Store) makeFolder(folder string) error {
	var created []string
	for p := folder; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil || filepath.Dir(p) == p {
			break
		}
		created = append(created, p)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	for _, p := range created {
		fixOwner(p)
	}
	return nil
}

// fileHead is what every file of ours starts with, to tell the two formats apart.
type fileHead struct {
	Format  string `json:"format"`
	Name    string `json:"name"`
	Note    string `json:"note"`
	SavedAt string `json:"savedAt"`
	Layers  []struct {
		Name string `json:"name"`
	} `json:"layers"`
}

func (s Store) read(dir, name string) ([]byte, fileHead, error) {
	var h fileHead
	if err := checkName(name); err != nil {
		return nil, h, err
	}
	data, err := os.ReadFile(filepath.Join(s.Root, dir, name))
	if err != nil {
		return nil, h, err
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, h, fmt.Errorf("%s is not valid JSON: %w", name, err)
	}
	return data, h, nil
}

// Load reads one layout from layouts/.
func (s Store) Load(name string) (Layout, error) {
	var l Layout
	data, h, err := s.read(LayoutsDir, name)
	if err != nil {
		return l, err
	}
	if h.Format != Format {
		return l, fmt.Errorf("%s is not a keypad layout file", name)
	}
	err = json.Unmarshal(data, &l)
	return l, err
}

// LoadBackup reads a backup. Old one-layer backups become a keypad with one layer.
func (s Store) LoadBackup(name string) (Keypad, error) {
	var k Keypad
	data, h, err := s.read(BackupsDir, name)
	if err != nil {
		return k, err
	}
	switch h.Format {
	case FormatKeypad:
		err = json.Unmarshal(data, &k)
	case Format:
		var l Layout
		if err = json.Unmarshal(data, &l); err == nil {
			k = Keypad{Format: FormatKeypad, Version: 1, Note: l.Note, SavedAt: l.SavedAt, Layers: []Layer{{Name: l.Name, Keys: l.Keys}}}
		}
	default:
		err = fmt.Errorf("%s is not a keypad backup", name)
	}
	return k, err
}

// Delete removes one layout file. Only layouts/ can be changed from the page.
func (s Store) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	return os.Remove(filepath.Join(s.Root, LayoutsDir, name))
}

// Rename gives a layout file a new name; it never replaces another file.
func (s Store) Rename(oldName, newName string) (string, error) {
	if err := checkName(oldName); err != nil {
		return "", err
	}
	newName, err := CleanName(newName)
	if err != nil {
		return "", err
	}
	if newName == oldName {
		return newName, nil // nothing to do
	}
	from, to := filepath.Join(s.Root, LayoutsDir, oldName), filepath.Join(s.Root, LayoutsDir, newName)
	fromInfo, err := os.Stat(from)
	if err != nil {
		return "", err
	}
	// "to" may exist as another file (taken), or be this very file under another letter
	// case on Windows / FAT (then only the case changes).
	if toInfo, err := os.Stat(to); err == nil && !os.SameFile(fromInfo, toInfo) {
		return "", fmt.Errorf("a layout named %s already exists", newName)
	}
	if strings.EqualFold(oldName, newName) {
		// Only the letter case changes: Windows needs a step in between.
		tmp := from + ".renaming"
		if err := os.Rename(from, tmp); err != nil {
			return "", err
		}
		return newName, os.Rename(tmp, to)
	}
	if err := os.Rename(from, to); err != nil {
		return "", err
	}
	return newName, nil
}

// FileInfo describes one stored file.
type FileInfo struct {
	Name    string   `json:"name"`
	Layers  []string `json:"layers"` // layer names inside the file
	Note    string   `json:"note,omitempty"`
	SavedAt string   `json:"savedAt"`
}

// List returns the files in dir, newest first. Files that are not ours are skipped.
func (s Store) List(dir string) []FileInfo {
	entries, _ := os.ReadDir(filepath.Join(s.Root, dir))
	out := []FileInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		_, h, err := s.read(dir, e.Name())
		if err != nil {
			continue
		}
		fi := FileInfo{Name: e.Name(), Note: h.Note, SavedAt: h.SavedAt}
		switch h.Format {
		case Format:
			fi.Layers = []string{h.Name}
		case FormatKeypad:
			for _, l := range h.Layers {
				fi.Layers = append(fi.Layers, l.Name)
			}
		default:
			continue
		}
		out = append(out, fi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt > out[j].SavedAt })
	return out
}

// CheckWritable proves the program folder takes files, so the page can warn in red when it does not.
func (s Store) CheckWritable() error {
	folder := filepath.Join(s.Root, BackupsDir)
	if err := s.makeFolder(folder); err != nil {
		return err
	}
	probe := filepath.Join(folder, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return err
	}
	return os.Remove(probe)
}
