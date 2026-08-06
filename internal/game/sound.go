package game

import (
	"ugataima/internal/config"
	"ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

type gameplaySound uint8

const (
	soundMeleeSwing gameplaySound = iota
	soundMonsterMeleeSwing
	soundMonsterHit
	soundPartyHit
	soundEnemyDeath
	soundDoorOpen
	soundDoorLocked
	soundChestOpen
	soundCoins
	soundQuestComplete
	soundLevelUp
	gameplaySoundCount
)

var gameplaySoundCatalogKeys = [gameplaySoundCount]string{
	"melee_swing",
	"monster_melee_swing",
	"monster_hit",
	"party_hit",
	"enemy_death",
	"door_open",
	"door_locked",
	"chest_open",
	"coins",
	"quest_complete",
	"level_up",
}

func (key gameplaySound) catalogKey() string {
	if key >= gameplaySoundCount {
		return ""
	}
	return gameplaySoundCatalogKeys[key]
}

// RequiredSoundKeys returns events referenced directly by game code. Routed
// school and weapon-category events are validated through audio.yaml.
func RequiredSoundKeys() []string {
	keys := append([]string(nil), gameplaySoundCatalogKeys[:]...)
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		seen[key] = struct{}{}
	}
	for _, key := range config.CrateSoundKeys() {
		if _, exists := seen[key]; exists {
			continue
		}
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	return keys
}

func RequiredSoundSchools() []string {
	schools := make([]string, 0, len(damage.Types())-1)
	for _, school := range damage.Types() {
		if school != damage.Physical {
			schools = append(schools, school.String())
		}
	}
	return schools
}

func RequiredWeaponSoundCategories() []string {
	return config.RangedWeaponSoundCategories()
}

func (g *MMGame) playSound(key gameplaySound) bool {
	return g.playSoundKey(key.catalogKey())
}

func (g *MMGame) playSoundKey(key string) bool {
	return g != nil && g.soundManager != nil && key != "" && g.soundManager.Play(key)
}

func (g *MMGame) playMonsterSound(key gameplaySound, target *monsterPkg.Monster3D) bool {
	if target == nil || g == nil || g.soundManager == nil {
		return false
	}
	return g.soundManager.PlayWithGain(key.catalogKey(), g.worldSoundGain(target.X, target.Y))
}

func (g *MMGame) worldSoundGain(x, y float64) float64 {
	if g == nil || g.camera == nil {
		return 0
	}
	maxDistance := g.camera.ViewDist
	if maxDistance <= 0 && g.config != nil {
		maxDistance = g.config.GetViewDistance()
	}
	if maxDistance <= 0 {
		return 0
	}
	distance := Distance(g.camera.X, g.camera.Y, x, y)
	if distance >= maxDistance {
		return 0
	}
	linear := 1 - distance/maxDistance
	return linear * linear
}

func (g *MMGame) playSpellSound(def spells.SpellDefinition) bool {
	return g != nil && g.soundManager != nil && g.soundManager.PlaySchoolWithGain(def.School, def.IsOffensive(), 1)
}

func (g *MMGame) playRangedWeaponAttackSound(def *config.WeaponDefinitionConfig) bool {
	return g.playRangedWeaponAttackSoundWithGain(def, 1)
}

func (g *MMGame) playMonsterSpellSound(def spells.SpellDefinition, source *monsterPkg.Monster3D) bool {
	return g.playMonsterSchoolSound(def.School, def.IsOffensive(), source)
}

func (g *MMGame) playMonsterSchoolSound(school string, offensive bool, source *monsterPkg.Monster3D) bool {
	if source == nil || g == nil || g.soundManager == nil {
		return false
	}
	return g.soundManager.PlaySchoolWithGain(school, offensive, g.worldSoundGain(source.X, source.Y))
}

func (g *MMGame) playMonsterRangedWeaponAttackSound(def *config.WeaponDefinitionConfig, source *monsterPkg.Monster3D) bool {
	if source == nil {
		return false
	}
	return g.playRangedWeaponAttackSoundWithGain(def, g.worldSoundGain(source.X, source.Y))
}

func (g *MMGame) playRangedWeaponAttackSoundWithGain(def *config.WeaponDefinitionConfig, gain float64) bool {
	if g == nil || g.soundManager == nil || gain <= 0 {
		return false
	}
	if config.IsMagicRangedWeapon(def) {
		return g.playMagicRangedWeaponAttackSoundWithGain(def, gain)
	}
	if def == nil {
		return false
	}
	return g.soundManager.PlayWeaponCategoryWithGain(def.Category, gain)
}

func (g *MMGame) playMagicRangedWeaponAttackSoundWithGain(def *config.WeaponDefinitionConfig, gain float64) bool {
	school, offensive := magicRangedWeaponAttackSoundRoute(def)
	if school == "" {
		return false
	}
	return g != nil && g.soundManager != nil && g.soundManager.PlaySchoolWithGain(school, offensive, gain)
}

func magicRangedWeaponAttackSoundRoute(def *config.WeaponDefinitionConfig) (school string, offensive bool) {
	if def == nil {
		return "", false
	}
	return def.ProjectileSchool, true
}

func (g *MMGame) updateLocationMusic() {
	if g == nil || g.soundManager == nil {
		return
	}
	biome := ""
	bossCombat := false
	if g.appScreen == AppScreenInGame && world.GlobalWorldManager != nil {
		if mapConfig := world.GlobalWorldManager.GetCurrentMapConfig(); mapConfig != nil {
			biome = mapConfig.Biome
		}
		if g.world != nil {
			bossCombat = bossMusicCombatActive(g.world.Monsters)
		}
	}
	g.soundManager.SetMusicState(biome, bossCombat)
}

func bossMusicCombatActive(monsters []*monsterPkg.Monster3D) bool {
	for _, candidate := range monsters {
		if candidate != nil && candidate.IsBoss() && candidate.IsAlive() &&
			(candidate.IsInCombat() || candidate.CurrentAIBehavior() == monsterPkg.AIBehaviorFleeing) {
			return true
		}
	}
	return false
}
