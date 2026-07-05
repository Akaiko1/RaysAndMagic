package game

// Debug module (not a regression test): loads the real app save and reports how
// much each party member's Space (SmartAttack) hit lands on every dragon type,
// normal and crit, after the actual armor + resistance pipeline. Assumes nobody
// is wounded (so Space doesn't heal) and casters can pay (SP topped up).
//
//	RAM_DRAGON=1 go test ./internal/game/ -run TestDebugDragonSpace -v

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestDebugDragonSpace(t *testing.T) {
	if os.Getenv("RAM_DRAGON") == "" {
		t.Skip("debug module; run with RAM_DRAGON=1")
	}
	t.Chdir("../..")

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatalf("traps: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("assets/level_up.yaml"); err != nil {
		t.Fatalf("levelup: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	monsterPkg.MustLoadMonsterConfig("assets/monsters.yaml")

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	world.GlobalWorldManager = wm

	savePath := os.Getenv("RAM_SAVE")
	if savePath == "" {
		savePath = os.ExpandEnv("$HOME/Library/Application Support/RaysAndMagic/saves/save1.json")
	}
	raw, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read save: %v", err)
	}
	var save GameSave
	if err := json.Unmarshal(raw, &save); err != nil {
		t.Fatalf("parse save: %v", err)
	}
	if err := wm.SwitchToMap(save.MapKey); err != nil {
		t.Fatalf("switch: %v", err)
	}
	g := newTestGame(cfg, wm.GetCurrentWorld())
	g.combat = NewCombatSystem(g)
	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	cs := g.combat
	camX, camY := g.camera.X, g.camera.Y

	dragons := []string{
		"dragon", "dragon_red", "dragon_green", "dragon_gold",
		"elder_dragon", "elder_dragon_red", "elder_dragon_green", "elder_dragon_gold",
	}
	freshDragon := func(key string) *monsterPkg.Monster3D {
		return monsterPkg.NewMonster3DFromConfig(camX+128, camY, key, cfg)
	}

	// armorReduce mirrors applyMonsterArmor for the deterministic (non-pierce)
	// case: elemental cap for non-physical schools, physical cap otherwise.
	armorReduce := func(rawDmg, ac int, physical bool) int {
		p := armorMitigationPctFromAC(ac, physical)
		if p <= 0 {
			return rawDmg
		}
		r := rawDmg * (100 - p) / 100
		if r < 1 {
			r = 1
		}
		return r
	}
	// resistReduce mirrors Monster3D.TakeDamageResist's resist step.
	resistReduce := func(dmg int, dt monsterPkg.DamageType, mon *monsterPkg.Monster3D, piercePct int) int {
		if resist, ok := mon.Resistances[dt]; ok {
			if piercePct > 0 && resist > 0 {
				resist = resist * (100 - piercePct) / 100
			}
			dmg = dmg * (100 - resist) / 100
			if dmg < 0 {
				dmg = 0
			}
		}
		return dmg
	}

	// weaponHit computes final damage of one weapon swing against a dragon.
	weaponHit := func(attacker *character.MMCharacter, weapon items.Item, rawDmg int, mon *monsterPkg.Monster3D) int {
		weaponDef := lookupWeaponConfigByName(weapon.Name)
		dtStr := weaponDamageTypeStr(weaponDef)
		dt := convertToMonsterDamageType(dtStr)
		isRanged := weaponDef != nil && weaponDef.Range > 3
		physical := isPhysicalDamageType(dtStr)
		reduced := armorReduce(rawDmg, mon.ArmorClass, physical)
		if mult := cs.weaponBonusMultiplier(weaponDef, mon); mult != 1.0 {
			reduced = int(math.Round(float64(reduced) * mult))
			if reduced < 1 {
				reduced = 1
			}
		}
		if mult := g.cardBonusVsMultiplier(mon); mult != 1.0 {
			reduced = int(math.Round(float64(reduced) * mult))
			if reduced < 1 {
				reduced = 1
			}
		}
		trueDmg, _ := cs.weaponMasteryStrike(attacker, weaponDef)
		reduced += trueDmg
		_ = isRanged
		return resistReduce(reduced, dt, mon, 0)
	}
	spellHit := func(caster *character.MMCharacter, spellID spells.SpellID, def spells.SpellDefinition, mon *monsterPkg.Monster3D) int {
		_, _, total := cs.CalculateSpellDamage(spellID, caster)
		dtStr := normalizeDamageTypeStr(def.School)
		dt := convertToMonsterDamageType(dtStr)
		reduced := armorReduce(total, mon.ArmorClass, isPhysicalDamageType(dtStr))
		pierce := cs.spellResistPierce(caster, string(spellID))
		return resistReduce(reduced, dt, mon, pierce)
	}

	// header
	hdr := "%-22s"
	line := fmt.Sprintf(hdr, "MEMBER / ACTION")
	for _, d := range dragons {
		line += fmt.Sprintf(" %-9s", d)
	}
	t.Log(line)

	for i, m := range g.party.Members {
		if m == nil {
			continue
		}
		g.selectedChar = i
		m.SpellPoints = m.MaxSpellPoints // model "can pay"

		// Space decision: offensive quick spell if slotted & affordable, else weapon.
		castSpell := false
		var spellID spells.SpellID
		var spellDef spells.SpellDefinition
		if sp, ok := m.Equipment[items.SlotSpell]; ok {
			id := spells.SpellID(sp.SpellEffect)
			if def, err := spells.GetSpellDefinitionByID(id); err == nil && def.IsOffensive() {
				if m.SpellPoints >= cs.effectiveSpellCost(m, sp.SpellCost) {
					castSpell, spellID, spellDef = true, id, def
				}
			}
		}

		if castSpell {
			row := fmt.Sprintf(hdr, fmt.Sprintf("%s: cast %s", m.Name, spellDef.Name))
			for _, dk := range dragons {
				row += fmt.Sprintf(" %-9d", spellHit(m, spellID, spellDef, freshDragon(dk)))
			}
			t.Logf("%s  (spells don't crit)", row)
			continue
		}

		// Weapon: report each hand that holds a weapon.
		hands := []items.EquipSlot{items.SlotMainHand, items.SlotOffHand}
		for _, slot := range hands {
			w, ok := m.Equipment[slot]
			if !ok || w.Type != items.ItemWeapon {
				continue
			}
			_, _, total := cs.CalculateWeaponDamage(w, m)
			hand := "main"
			if slot == items.SlotOffHand {
				hand = "off"
			}
			normRow := fmt.Sprintf(hdr, fmt.Sprintf("%s: %s(%s)", m.Name, w.Name, hand))
			critRow := fmt.Sprintf(hdr, "  -> crit")
			for _, dk := range dragons {
				normRow += fmt.Sprintf(" %-9d", weaponHit(m, w, total, freshDragon(dk)))
			}
			for _, dk := range dragons {
				critRow += fmt.Sprintf(" %-9d", weaponHit(m, w, total*CritDamageMultiplier, freshDragon(dk)))
			}
			t.Logf("%s  (raw %d)", normRow, total)
			t.Log(critRow)
		}
	}
	t.Log("NOTE: bow hits have a 33% chance to ignore armor (higher than shown); Monk fists add a Spiritual Training free-spell proc by chance; weapon-mastery true damage is included but still passes through the dragon's resistance.")
}
