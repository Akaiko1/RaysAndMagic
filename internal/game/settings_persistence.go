package game

import (
	"encoding/json"
	"log"
	"os"
	"ugataima/internal/storage"
)

// Read into a temporary value so malformed JSON never partially enables a setting.
func loadSettingsJSON[T any](path, label string) (T, bool) {
	var value T
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return value, false
	}
	if err == nil {
		err = json.Unmarshal(raw, &value)
	}
	if err != nil {
		log.Printf("Could not load %s settings: %v", label, err)
		var zero T
		return zero, false
	}
	return value, true
}

// Each file retains its own failure until that same save succeeds.
func (g *MMGame) saveSettingsJSON(path, label string, value any, dirty *bool) {
	if !*dirty {
		return
	}
	err := storage.WriteJSONAtomic(path, value, 0600)
	if err == nil {
		*dirty = false
	}
	g.recordSettingsSave(path, label, err)
}

func (g *MMGame) recordSettingsSave(path, label string, err error) {
	if g.settingsSaveFailures == nil {
		g.settingsSaveFailures = make(map[string]bool)
	}
	if err != nil {
		g.settingsSaveFailures[path] = true
		log.Printf("Could not save %s settings: %v", label, err)
	} else {
		delete(g.settingsSaveFailures, path)
	}

	g.settingsSaveError = ""
	if len(g.settingsSaveFailures) > 0 {
		g.settingsSaveError = "Could not save settings. Please check the data folder."
	}
}
