package game

import (
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// A summoned ally knows the hero who called it - recorded when it is summoned
// and kept through a save - and wears that hero's face over its head. Card
// allies (the whole party's), bound former enemies and an ally whose summoner
// has left the party have no summoner.
func TestSummonerOfEveryAllyKind(t *testing.T) {
	for _, tc := range []struct {
		name   string
		summon func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter)
	}{
		{"summon spell names its caster", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			def, err := spells.GetSpellDefinitionByID("summon_ice_elemental")
			if err != nil {
				t.Fatal(err)
			}
			caster := g.party.Members[1]
			if !g.combat.tryCastSummon(def, caster).handled() {
				t.Fatal("summon spell was not handled")
			}
			return g.world.Monsters[len(g.world.Monsters)-1], caster
		}},
		{"animal bonding names its druid", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			druid := character.CreateCharacter("Rowan", character.ClassDruid, g.config)
			druid.Skills[character.SkillAnimalBonding].Mastery = character.MasteryMaster
			g.party.Members[2] = druid
			if !g.combat.summonAnimalBondingBear(druid) {
				t.Fatal("no bear placed")
			}
			return g.world.Monsters[len(g.world.Monsters)-1], druid
		}},
		{"bear from an older save", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			druid := g.party.Members[3]
			bear := g.combat.spawnPartyAlly("bear", animalBondingOwner(druid))
			bear.SummonerName = ""
			return bear, druid
		}},
		{"card ally is the party's", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			return g.combat.spawnPartyAlly("masked_huntress", cardSummonOwnerPrefix+"7"), nil
		}},
		{"bound former enemy", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			m := monsterPkg.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "goblin", g.config)
			m.Bound = true
			g.world.Monsters = append(g.world.Monsters, m)
			return m, nil
		}},
		{"summoner left the party", func(t *testing.T, g *MMGame) (*monsterPkg.Monster3D, *character.MMCharacter) {
			ally := g.combat.spawnPartyAlly("bear", animalBondingOwner(g.party.Members[0]))
			ally.SummonerName = "Someone Retired"
			return ally, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			ally, want := tc.summon(t, g)
			if ally == nil {
				t.Fatal("fixture: nothing summoned")
			}
			if got := g.summonerOf(ally); got != want {
				t.Fatalf("summoner %v, want %v", got, want)
			}
			if want == nil {
				return
			}
			// A save keeps the summoner.
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
			wm.CurrentMapKey = "forest"
			save := g.buildSave(wm)
			save.MapKey = "forest"
			wLoad := newTestWorld(g.config)
			wmLoad := world.NewWorldManager(g.config)
			wmLoad.LoadedMaps = map[string]*world.World3D{"forest": wLoad}
			wmLoad.CurrentMapKey = "forest"
			gLoad := newTestGame(g.config, wLoad)
			gLoad.party = g.party
			if err := gLoad.applySave(wmLoad, &save); err != nil {
				t.Fatalf("apply save: %v", err)
			}
			for _, m := range wLoad.Monsters {
				if m.ID == ally.ID {
					if got := gLoad.summonerOf(m); got == nil || got.Name != want.Name {
						t.Fatalf("after load the summoner is %v, want %s", got, want.Name)
					}
					return
				}
			}
			t.Fatal("the ally did not survive the save")
		})
	}
}
