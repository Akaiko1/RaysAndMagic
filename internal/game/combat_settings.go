package game

import "ugataima/internal/storage"

type combatPreferences struct {
	TurnBasedOverlay bool `json:"turn_based_overlay"`
	WideView         bool `json:"wide_view"`
}

func combatPreferencesPath() string { return storage.AppSavePath("combat_settings.json") }

func (g *MMGame) loadCombatPreferences() {
	g.combatPreferences = combatPreferences{}
	if prefs, ok := loadSettingsJSON[combatPreferences](combatPreferencesPath(), "combat"); ok {
		g.combatPreferences = prefs
	}
}

func (g *MMGame) saveCombatPreferences() {
	g.saveSettingsJSON(combatPreferencesPath(), "combat", g.combatPreferences, &g.combatSettingsDirty)
}

func (g *MMGame) setCombatOverlay(enabled bool) {
	if g.combatPreferences.TurnBasedOverlay != enabled {
		g.combatPreferences.TurnBasedOverlay = enabled
		g.combatSettingsDirty = true
	}
	g.saveCombatPreferences()
}

func (g *MMGame) setWideView(wide bool) {
	if g.combatPreferences.WideView != wide {
		g.combatPreferences.WideView = wide
		g.combatSettingsDirty = true
		g.resetCameraPresentation()
	}
	g.saveCombatPreferences()
}
