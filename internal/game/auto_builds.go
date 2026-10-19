package game

import (
	"cmp"
	"fmt"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stats"
)

// autoBuildPreviewLevels are the milestones Settings previews a build at.
var autoBuildPreviewLevels = [3]int{15, 30, 45}

// autoBuildPreview is one build of a class as a hero leveled by it from level
// 1 stands at each preview level, and the gear the build is made for.
type autoBuildPreview struct {
	build  config.AutoBuild
	stats  [len(autoBuildPreviewLevels)][]int // in stats.Names order
	hp, sp [len(autoBuildPreviewLevels)]int
	gear   []items.Item
}

// classAutoBuilds is every AUTO build a class offers, its default first.
func classAutoBuilds(class character.CharacterClass, cfg *config.Config) []config.AutoBuild {
	return cfg.Characters.Classes[class.Key()].AutoBuildOptions()
}

// autoBuildFor is the build the party AUTO button follows for heroes of class:
// the one chosen in Settings, or the class default when none is chosen or the
// chosen build is no longer authored.
func (g *MMGame) autoBuildFor(class character.CharacterClass) config.AutoBuild {
	c := g.config.Characters.Classes[class.Key()]
	key := g.combatPreferences.AutoBuilds[class.Key()]
	for _, b := range c.AutoBuilds {
		if b.Key == key {
			return b
		}
	}
	return c.DefaultAutoBuild()
}

// autoBuildHero is a fresh hero of class leveled to level with every point
// spent by build.
func autoBuildHero(class character.CharacterClass, build config.AutoStatsConfig, level int, cfg *config.Config) *character.MMCharacter {
	ch := character.CreateCharacter("Preview", class, cfg)
	ch.Level = level
	ch.FreeStatPoints = (level - 1) * StatPointsPerLevel
	autoDistributeStatPoints(ch, build, cfg)
	ch.CalculateDerivedStats(cfg)
	return ch
}

// autoBuildGearItem resolves a gear key from weapons.yaml or items.yaml.
func autoBuildGearItem(key string) (items.Item, *config.WeaponDefinitionConfig, bool) {
	if def, ok := config.GlobalWeapons.Weapons[key]; ok && def != nil {
		return items.CreateWeaponFromYAML(key), def, true
	}
	if def, ok := config.GetItemDefinition(key); ok && def != nil && (def.Type == "armor" || def.Type == "accessory") {
		it, err := items.TryCreateItemFromYAML(key)
		return it, nil, err == nil
	}
	return items.Item{}, nil, false
}

// autoBuildPreviews is computed once per class: authored data does not change
// while the game runs.
func (g *MMGame) autoBuildPreviews(class character.CharacterClass) []autoBuildPreview {
	if previews, ok := g.autoBuildPreviewCache[class]; ok {
		return previews
	}
	var previews []autoBuildPreview
	for _, b := range classAutoBuilds(class, g.config) {
		p := autoBuildPreview{build: b}
		for i, level := range autoBuildPreviewLevels {
			ch := autoBuildHero(class, b.AutoStatsConfig, level, g.config)
			for _, name := range stats.Names {
				p.stats[i] = append(p.stats[i], *ch.BaseStatField(name))
			}
			p.hp[i], p.sp[i] = ch.MaxHitPoints, ch.MaxSpellPoints
		}
		for _, key := range b.Gear {
			it, _, _ := autoBuildGearItem(key)
			p.gear = append(p.gear, it)
		}
		previews = append(previews, p)
	}
	if g.autoBuildPreviewCache == nil {
		g.autoBuildPreviewCache = map[character.CharacterClass][]autoBuildPreview{}
	}
	g.autoBuildPreviewCache[class] = previews
	return previews
}

// autoBuildClasses are the classes Settings offers builds for: those of the
// party in play, or every playable class before a game starts.
func (g *MMGame) autoBuildClasses() []character.CharacterClass {
	if g.appScreen != AppScreenInGame || g.party == nil {
		return character.PlayableClasses
	}
	var out []character.CharacterClass
	for _, m := range g.party.Members {
		if m != nil && !slicesContainsClass(out, m.Class) {
			out = append(out, m.Class)
		}
	}
	return out
}

func slicesContainsClass(classes []character.CharacterClass, c character.CharacterClass) bool {
	for _, x := range classes {
		if x == c {
			return true
		}
	}
	return false
}

// validateAutoBuildGear checks every build's gear at boot: each piece exists,
// a hero of the class leveled in the build can use it, and it serves the build
// (a weapon scales with its primary or secondary stat; wear raises one of
// them, Endurance or Speed).
func validateAutoBuildGear(cfg *config.Config) {
	for _, class := range character.PlayableClasses {
		for _, b := range classAutoBuilds(class, cfg) {
			hero := autoBuildHero(class, b.AutoStatsConfig, autoBuildPreviewLevels[len(autoBuildPreviewLevels)-1], cfg)
			for _, key := range b.Gear {
				it, weapon, ok := autoBuildGearItem(key)
				fail := func(why string) {
					panic(fmt.Sprintf("class %q auto build %q: gear %q %s", class.Key(), b.Key, key, why))
				}
				switch {
				case !ok:
					fail("is not a weapon, armor or accessory")
				case weapon != nil:
					if !hero.CanEquipWeapon(it) {
						fail("cannot be wielded by the class")
					}
					if stat := strings.ToLower(weapon.BonusStat); stat == "" || stat != b.Primary && stat != b.Secondary {
						fail("scales with " + cmp.Or(weapon.BonusStat, "no stat") + ", not the build's stats")
					}
				default:
					slot := it.PreferredSlot(items.SlotArmor)
					if it.Type == items.ItemAccessory {
						slot = it.PreferredSlot(items.SlotRing1)
					}
					if !hero.ItemFitsSlot(it, slot) {
						fail("cannot be worn by the class")
					}
					bonus := func(stat string) int { return it.Attributes["bonus_"+stat] }
					if bonus(b.Primary)+bonus(b.Secondary)+bonus("endurance")+bonus("speed") <= 0 {
						fail("raises none of the build's stats, Endurance or Speed")
					}
				}
			}
		}
	}
}
