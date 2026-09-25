package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"ninekeypad/internal/layout"
)

// SettingsFile sits next to the program and keeps the page address the same between starts,
// so it can be bookmarked. It holds the secret key of the page address.
const SettingsFile = "ninekeypad-settings.json"

// oldSettingsFile is the name used before the program was called ninekeypad; it is still
// read, so an existing bookmark keeps working.
const oldSettingsFile = "keypad-settings.json"

// DefaultPort is tried first on a fresh start.
const DefaultPort = 47821

type Settings struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

var tokenRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// LoadSettings reads the settings file; a missing or broken file gives fresh settings.
// changed = true means the caller should save them (fresh, or found under the old name).
func LoadSettings(root string) (s Settings, changed bool) {
	for i, name := range []string{SettingsFile, oldSettingsFile} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && json.Unmarshal(data, &s) == nil && s.Port > 0 && s.Port < 65536 && tokenRe.MatchString(s.Token) {
			return s, i > 0
		}
		s = Settings{}
	}
	return Settings{Port: DefaultPort, Token: NewToken()}, true
}

// SaveSettings writes the settings readable by the owner only: the file holds the secret key.
func SaveSettings(root string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return layout.Store{Root: root}.WriteFile(SettingsFile, append(data, '\n'), 0o600)
}

// Token is the secret key of the page address.
func (s *Server) Token() string { return s.token }
