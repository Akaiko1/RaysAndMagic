package game

import (
	"log"
	"ugataima/internal/config"
	"ugataima/internal/storage"
)

func potionPreferencesPath() string { return storage.AppSavePath("potion_settings.json") }
func (g *MMGame) loadPotionPreferences() {
	if err := g.config.LoadPotionPreferences(potionPreferencesPath()); err != nil {
		log.Printf("Could not load potion settings: %v", err)
		g.config.EnsurePotionPreferences()
	}
}
func (g *MMGame) savePotionPreferences() {
	if !g.potionSettingsDirty || g.config == nil {
		return
	}
	if err := storage.WriteJSONAtomic(potionPreferencesPath(), g.config.EnsurePotionPreferences(), 0600); err != nil {
		g.settingsSaveError = "Could not save settings. Please check the data folder."
		log.Printf("Could not save potion settings: %v", err)
		return
	}
	g.potionSettingsDirty = false
	g.settingsSaveError = ""
}
func (g *MMGame) setPotionThreshold(mana bool, percent int) {
	if g.config == nil {
		return
	}
	p := g.config.EnsurePotionPreferences().Resource(mana)
	percent = max(0, min(100, percent))
	if p.ThresholdPct != percent {
		p.ThresholdPct = percent
		g.potionSettingsDirty = true
	}
}
func (g *MMGame) toggleAutomaticPotion(mana bool, key string) {
	def, ok := config.GetItemDefinition(key)
	if !ok || !config.IsAutomaticPotion(def, mana) || def.BrewedFrom != "" {
		return
	}
	p := g.config.EnsurePotionPreferences().Resource(mana)
	p.Allowed[key] = !p.Allowed[key]
	g.potionSettingsDirty = true
	g.savePotionPreferences()
}
