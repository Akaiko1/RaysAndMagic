package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"ugataima/internal/arena"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// championTemplates caches the read-only character build per champion key. The
// live damage path and the static mirror both read it; it is never mutated, so
// one shared instance per key is safe. Primed at startup (PrimeChampions) for
// fail-fast validation, then read at runtime.
var championTemplates = map[string]*character.MMCharacter{}

// championTierOf resolves a champion mob's difficulty tier name (default for
// pre-placed spawns and older saves).
func championTierOf(m *monster.Monster3D) string {
	if m != nil && m.ChampionTier != "" {
		return m.ChampionTier
	}
	return config.ChampionDefaultTier
}

// buildChampionTemplate constructs the full champion character at one tier:
// the authored build (class/race/skills/equipment) at the tier's level and
// mastery, stat points spent by the SAME per-class auto-distribution the party
// uses, then derived stats recomputed - so every stat/gear/skill bonus a real
// character gets is present on the template.
func buildChampionTemplate(def *config.ChampionDefinition, tierName string, cfg *config.Config) (*character.MMCharacter, error) {
	tier := config.GetChampionTier(tierName)
	if tier == nil {
		return nil, fmt.Errorf("champion tier %q not configured", tierName)
	}
	ch, err := character.BuildChampion(def, tier, tierName, cfg)
	if err != nil {
		return nil, err
	}
	if ch.Level > 1 {
		ch.FreeStatPoints = (ch.Level - 1) * StatPointsPerLevel
		autoDistributeStatPoints(ch, cfg)
	}
	ch.CalculateDerivedStats(cfg)
	return ch, nil
}

func championTemplateKey(key, tierName string) string { return key + "|" + tierName }

// PrimeChampions builds and caches every champion at every tier, returning the
// first build error. Called at startup so a malformed build (bad key, gear the
// skills can't wear) fails loud instead of shipping a gimped champion.
func PrimeChampions(cfg *config.Config) error {
	if config.GlobalChampionConfig == nil {
		return nil
	}
	for key, def := range config.GlobalChampionConfig.Champions {
		if err := validateChampionSpells(key, def); err != nil {
			return err
		}
		for tierName := range config.GlobalChampionConfig.Tiers {
			ch, err := buildChampionTemplate(def, tierName, cfg)
			if err != nil {
				return err
			}
			championTemplates[championTemplateKey(key, tierName)] = ch
		}
	}
	return nil
}

// championTemplate returns the cached character build for a champion at a
// tier, building it on demand if the cache is cold (validated at startup).
func (g *MMGame) championTemplate(key, tierName string) *character.MMCharacter {
	tk := championTemplateKey(key, tierName)
	if ch, ok := championTemplates[tk]; ok {
		return ch
	}
	def := config.GetChampionDefinition(key)
	if def == nil {
		return nil
	}
	ch, err := buildChampionTemplate(def, tierName, g.config)
	if err != nil {
		return nil
	}
	championTemplates[tk] = ch
	return ch
}

// championTemplateFor resolves the template for a live champion mob.
func (g *MMGame) championTemplateFor(m *monster.Monster3D) *character.MMCharacter {
	return g.championTemplate(m.ChampionKey, championTierOf(m))
}

// livingChampion returns a living champion mob on the current map, or nil.
// The ONE definition of "a duel is underway" - doors and the duel guard share it.
func (g *MMGame) livingChampion() *monster.Monster3D {
	for _, m := range g.world.Monsters {
		if m != nil && m.IsChampion() && m.IsAlive() {
			return m
		}
	}
	return nil
}

// championOffHandWeapon returns the build's off-hand weapon, if it wields one.
func championOffHandWeapon(ch *character.MMCharacter) (items.Item, bool) {
	off, ok := ch.Equipment[items.SlotOffHand]
	return off, ok && off.Type == items.ItemWeapon
}

// championHandWeapon resolves the striking hand's weapon; a mono-wielder's
// "off hand" falls back to the main hand.
func championHandWeapon(ch *character.MMCharacter, offHand bool) items.Item {
	if offHand {
		if off, ok := championOffHandWeapon(ch); ok {
			return off
		}
	}
	return ch.Equipment[items.SlotMainHand]
}

// applyChampionHandRiders stamps the STRIKING weapon's per-hit effects onto the
// mob just before its hits resolve: weapon-mastery true damage and GM
// dodge-pierce for that weapon's category, plus the weapon's own YAML on-hit
// riders. Stun seconds follow the weapon-stun convention of one second per TB
// turn (see config.go weapon stun doc / tryApplyWeaponStun).
func (cs *CombatSystem) applyChampionHandRiders(m *monster.Monster3D, ch *character.MMCharacter, wd *config.WeaponDefinitionConfig) {
	if wd == nil {
		return
	}
	trueDmg, ignoreDodge := cs.weaponMasteryStrike(ch, wd)
	m.TrueDamage = trueDmg
	m.IgnoresDodge = ignoreDodge
	m.StunCharChance = wd.StunChance
	m.StunCharTurns = wd.StunTurns
	m.StunCharSeconds = wd.StunTurns
}

// championSwingDamage is the ONE champion hit formula: stamp the striking
// weapon's riders, then roll damage through the character pipeline (weapon +
// effective stats + crit). Every champion attack path funnels through it.
func (cs *CombatSystem) championSwingDamage(m *monster.Monster3D, ch *character.MMCharacter, weapon items.Item) (*config.WeaponDefinitionConfig, int) {
	wd := lookupWeaponConfigByName(weapon.Name)
	cs.applyChampionHandRiders(m, ch, wd)
	_, _, total := cs.CalculateWeaponDamage(weapon, ch)
	if crit, _ := cs.RollWeaponCriticalChance(weapon, ch); crit {
		total *= CritDamageMultiplier
	}
	if total < 1 {
		total = 1
	}
	return wd, total
}

// championAlternatingStrike is the TURN-BASED melee entry: strict hand
// alternation across swings (main, off, main, off - party NextTBAttackOffHand
// parity). Mono-wielders always swing the main hand.
func (cs *CombatSystem) championAlternatingStrike(m *monster.Monster3D) bool {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		return false
	}
	if cs.championTryCastSpell(m) { // a melee caster's swing can become a cast too
		return true
	}
	off := false
	if _, dual := championOffHandWeapon(ch); dual {
		off = m.NextHandOff
		m.NextHandOff = !m.NextHandOff
	}
	return cs.championMeleeStrike(m, off)
}

// championMeleeStrike is a champion's melee swing against the party with the
// given hand: one damage roll, then the weapon's arc decides how many members
// the sweep catches (arc_type 1=single, 2=two, 3=three, 4=everyone - the
// party-side swing widths translated to a formation). An AoE-rider weapon
// instead sweeps the WHOLE party exactly once per swing: unlike the party's
// splash-per-arc-hit, a champion's arc and AoE never multiply.
func (cs *CombatSystem) championMeleeStrike(m *monster.Monster3D, offHand bool) bool {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		return false
	}
	wd, dmg := cs.championSwingDamage(m, ch, championHandWeapon(ch, offHand))
	dtype := "physical"
	if wd != nil && wd.DamageType != "" {
		dtype = wd.DamageType
	}
	if wd != nil && wd.AoeRadiusTiles > 0 {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's sweep engulfs the whole party!", m.Name))
		cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
			cs.monsterHitCharacter(m, member, m.Name, dmg, dtype, m.IgnoresArmor, 0, true)
		})
		return true
	}
	n := 1
	if wd != nil && wd.Melee != nil {
		switch wd.Melee.ArcType {
		case 2:
			n = 2
		case 3:
			n = 3
		case 4:
			n = len(cs.game.party.Members) // widest arc catches the whole formation
		}
	}
	targets := cs.randomLivingMembers(n)
	for _, t := range targets {
		cs.monsterHitCharacter(m, t, m.Name, dmg, dtype, m.IgnoresArmor, 0, true)
	}
	return len(targets) > 0
}

// championCrossfireStrike is one selected hand's swing at a bound-ally FOE
// (summon), giving that swing the SAME weapon mechanics the party gets in PvE:
// one damage roll spread across the arc/AoE it catches among summons, plus - as
// an ADDITIONAL action, when the same swing's geometry reaches the party - the
// champion's normal vs-party hit (championMeleeStrike, untouched). AoE never
// re-rolls and never stacks with the arc (the weapon is one or the other).
func (cs *CombatSystem) championCrossfireStrike(m *monster.Monster3D, foe *monster.Monster3D, offHand bool) {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		cs.monsterStrikeMonster(m, foe) // fallback: plain blow
		return
	}
	weapon := championHandWeapon(ch, offHand)
	wd, dmg := cs.championSwingDamage(m, ch, weapon)
	dtype := monster.DamagePhysical
	if wd != nil && wd.DamageType != "" {
		dtype = convertToMonsterDamageType(wd.DamageType)
	}
	ts := float64(cs.game.config.GetTileSize())
	facing := math.Atan2(foe.Y-m.Y, foe.X-m.X)
	partyCaught := false

	if wd != nil && wd.AoeRadiusTiles > 0 {
		// AoE: every bound-ally summon within the radius, and (as the extra action)
		// the party if it stands in the same radius.
		r := wd.AoeRadiusTiles * ts
		for _, o := range cs.game.world.Monsters {
			if o != nil && o.Bound && o.IsAlive() && Distance(m.X, m.Y, o.X, o.Y) <= r {
				cs.strikeMonsterFor(m, o, dmg, dtype)
			}
		}
		partyCaught = Distance(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y) <= r
	} else {
		// Arc: the weapon's cone catches summons AND, if it reaches the party
		// point, the party too - resolved by the shared applyMeleeArc.
		rangeTiles := 1
		arc := 1
		if wd != nil {
			if wd.Range > 0 {
				rangeTiles = wd.Range
			}
			if wd.Melee != nil {
				arc = wd.Melee.ArcType
			}
		}
		var cands []meleeArcCandidate
		for _, o := range cs.game.world.Monsters {
			if o == nil || !o.Bound || !o.IsAlive() {
				continue
			}
			if ang, ok := meleeReachAngle(m.X, m.Y, facing, rangeTiles, ts, o.X, o.Y); ok {
				summon := o
				cands = append(cands, meleeArcCandidate{ang: ang, hit: func() { cs.strikeMonsterFor(m, summon, dmg, dtype) }})
			}
		}
		if ang, ok := meleeReachAngle(m.X, m.Y, facing, rangeTiles, ts, cs.game.camera.X, cs.game.camera.Y); ok {
			cands = append(cands, meleeArcCandidate{ang: ang, hit: func() { partyCaught = true }})
		}
		applyMeleeArc(cands, arc)
	}

	// Party caught in the same sweep: the champion's normal vs-party hit (whole
	// party for AoE, arc_type members for an arc) - reused as-is.
	if partyCaught {
		cs.championMeleeStrike(m, false)
	}
}

// championAlternatingCrossfireStrike is the TB counterpart of the party's
// strict hand alternation: one monster turn uses one hand, then the other.
func (cs *CombatSystem) championAlternatingCrossfireStrike(m, foe *monster.Monster3D) {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		cs.monsterStrikeMonster(m, foe)
		return
	}
	offHand := false
	if _, dual := championOffHandWeapon(ch); dual {
		offHand = m.NextHandOff
		m.NextHandOff = !m.NextHandOff
	}
	cs.championCrossfireStrike(m, foe, offHand)
}

// championRTCrossfireStrike gives a champion fighting a bound ally the same
// independent hand cooldowns it has against the party. A fixed crossfire
// cadence would throttle Weapon Master to one main-hand swing per second and
// silently disable his off hand against summons.
func (cs *CombatSystem) championRTCrossfireStrike(m, foe *monster.Monster3D) bool {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		return false
	}
	if m.HasRangedAttack() {
		if m.AttackCDFrames == 0 {
			m.AttackCDFrames = m.AttackCooldownFrames()
			m.AttackAnimFrames = MonsterAttackAnimFrames
			cs.spawnMonsterRangedAttackAtMonster(m, foe, ProjectileOwnerMonsterAtBound)
		}
		return true
	}

	struck := false
	if m.AttackCDFrames == 0 {
		m.AttackCDFrames = m.AttackCooldownFrames()
		cs.championCrossfireStrike(m, foe, false)
		struck = true
	}
	if _, dual := championOffHandWeapon(ch); dual && m.OffHandCDFrames == 0 && foe.IsAlive() {
		m.OffHandCDFrames = cs.OffHandWeaponCooldownFrames(ch)
		cs.championCrossfireStrike(m, foe, true)
		struck = true
	}
	if struck {
		m.AttackAnimFrames = MonsterAttackAnimFrames
	}
	return true
}

// monsterAttackDamage is the ONE damage source for every monster attack site
// (projectiles, breath, piercing, monster-vs-monster; champion MELEE resolves
// through championMeleeStrike instead, which also picks the hand). Champions
// roll through championSwingDamage with their main hand (the ranged weapon);
// plain monsters keep their authored damage band.
func (cs *CombatSystem) monsterAttackDamage(m *monster.Monster3D) int {
	if m != nil && m.IsChampion() && cs.game != nil {
		if ch := cs.game.championTemplateFor(m); ch != nil {
			_, total := cs.championSwingDamage(m, ch, ch.Equipment[items.SlotMainHand])
			return total
		}
	}
	return m.GetAttackDamage()
}

// championRTDualStrike runs a melee champion's REAL-TIME attack moment with
// party dual-wield parity: two independent hand streams, each on its own
// weapon's cooldown (WeaponCooldownFrames / OffHandWeaponCooldownFrames - the
// same formulas the party uses). The main hand fires on the AI's attack tick
// (StateTimer==1, AttackCDFrames gate); the off hand fires whenever ITS
// cooldown has elapsed while the champion is in contact, independent of the
// state-machine tick - so a fast off-hand weapon genuinely swings faster.
// Returns whether this call handled the champion's attack logic.
func (cs *CombatSystem) championRTDualStrike(m *monster.Monster3D, attackTick bool) bool {
	ch := cs.game.championTemplateFor(m)
	if ch == nil {
		return false
	}
	struck := false
	if attackTick && m.AttackCDFrames == 0 {
		m.AttackCDFrames = m.AttackCooldownFrames()
		if !cs.championTryCastSpell(m) { // a melee caster's swing can become a cast
			cs.championMeleeStrike(m, false)
		}
		struck = true
	}
	if _, dual := championOffHandWeapon(ch); dual && m.OffHandCDFrames == 0 {
		m.OffHandCDFrames = cs.OffHandWeaponCooldownFrames(ch)
		cs.championMeleeStrike(m, true)
		struck = true
	}
	if struck {
		m.AttackAnimFrames = MonsterAttackAnimFrames
	}
	return true
}

// stampChampionProjectileRiders re-arms a champion's on-hit riders from the
// weapon that actually FIRED the projectile (its BowKey), at impact time - a
// swing landing while darts were in flight may have re-stamped the mob's rider
// fields for another hand.
func (cs *CombatSystem) stampChampionProjectileRiders(src *monster.Monster3D, bowKey string) {
	if src == nil || !src.IsChampion() || bowKey == "" {
		return
	}
	ch := cs.game.championTemplateFor(src)
	if ch == nil {
		return
	}
	if wd, ok := config.GetWeaponDefinition(bowKey); ok && wd != nil {
		cs.applyChampionHandRiders(src, ch, wd)
	}
}

// mirrorChampionStats stamps the STATIC character-derived numbers onto the
// champion's monster ONCE per instance (fresh spawn and save-load both rebuild
// the struct, resetting ChampionMirrored): real-time attack cadence (Speed +
// weapon + dual-wield), two turn-based swings, main-hand weapon reach and
// riders, ranged main-hand projectile (range from the weapon's own physics),
// armor class, gear resistances joining the authored table, and the
// character's perfect-dodge chance. Per-hit damage is live - see
// championSwingDamage. The HP pool and victory experience come from the
// difficulty TIER: MaxHitPoints = tier hp with current HP clamped (a fresh
// spawn starts at the mob's authored ceiling and clamps down to full; a loaded
// champion keeps its restored wounds).
func (g *MMGame) mirrorChampionStats(m *monster.Monster3D) {
	if m == nil || m.ChampionMirrored || g.combat == nil {
		return
	}
	ch := g.championTemplateFor(m)
	if ch == nil {
		return
	}
	cs := g.combat
	ts := float64(g.config.GetTileSize())

	if tier := config.GetChampionTier(championTierOf(m)); tier != nil {
		m.MaxHitPoints = tier.HP
		if m.HitPoints > m.MaxHitPoints {
			m.HitPoints = m.MaxHitPoints
		}
		m.Experience = tier.Experience
	}

	weapon := ch.Equipment[items.SlotMainHand]
	wd, _, found := config.GetWeaponDefinitionByName(weapon.Name)
	if found && wd != nil {
		cs.applyChampionHandRiders(m, ch, wd) // main-hand defaults until the first swing re-arms
		if wd.Range > 0 {
			m.AttackRadius = float64(wd.Range) * ts // weapon reach, like the party's melee arc
		}
	}

	// Real-time cadence: choose the cooldown multiplier so AttackCooldownFrames()
	// resolves to the character's weapon cooldown (Speed + weapon + dual-wield).
	if base := g.config.MonsterAI.AttackCooldown; base > 0 {
		m.AttackCooldownMultiplier = float64(cs.WeaponCooldownFrames(ch)) / float64(base)
	}
	m.AttacksPerRound = 2 // champions always get two turn-based swings

	// Ranged champions loose their MAIN-HAND weapon as a projectile; range then
	// comes from the weapon's own physics (GetAttackRangePixels), like the party.
	if def := config.GetChampionDefinition(m.ChampionKey); def != nil && def.Ranged {
		if _, key, ok := config.GetWeaponDefinitionByName(weapon.Name); ok {
			m.ProjectileWeapon = key
			m.RangedAttackRange = 0
		}
	}

	m.ArmorClass = cs.CalculateTotalArmorClass(ch)
	m.PerfectDodge = ch.GetEffectiveLuck()/LuckToDodgeDivisor + cs.armorGMDodgeBonus(ch)

	// Gear resistances (resist_<school> item attributes) ADD to the mob's
	// authored table. Safe to add: this runs once per instance.
	if monster.MonsterConfig != nil {
		for school := range monster.MonsterConfig.DamageTypes {
			if pct := ch.GearResistPct(school); pct != 0 {
				if dt, err := monster.MonsterConfig.ConvertDamageType(school); err == nil {
					m.Resistances[dt] += pct
				}
			}
		}
	}
	m.ChampionMirrored = true
}

// recordChampionVictory pays the tier's arena points and writes the global
// leaderboard entry (party snapshot + the champion/tier kill). Called from the
// kill choke point; XP flows through the normal award path (m.Experience is
// tier-mirrored).
func (cs *CombatSystem) recordChampionVictory(m *monster.Monster3D) {
	tierName := championTierOf(m)
	tier := config.GetChampionTier(tierName)
	if tier == nil {
		return
	}
	cs.game.awardArenaPoints(tier.ArenaPoints)
	cs.game.AddCombatMessage(fmt.Sprintf("%s falls! The crowd roars: +%d arena points.", m.Name, tier.ArenaPoints))

	members := make([]arena.Member, 0, len(cs.game.party.Members))
	for _, mem := range cs.game.party.Members {
		if mem != nil {
			members = append(members, arena.Member{Name: mem.Name, Class: mem.Class.String(), Level: mem.Level})
		}
	}
	if !arena.RecordVictory(cs.game.playthroughID, members, m.Name, tierName, tier.ArenaPoints, cs.game.dayNightDay) {
		cs.game.AddCombatMessage("The board already honors this day's victory.")
	}
	cs.game.arenaBoardStale = true
	cs.rollChampionSetDrops(m)
}

// rollChampionSetDrops rolls the armor-set trophy a fallen champion can yield
// (any tier): each SET listed in champions.yaml `set_drops:` rolls ONCE in turn
// at its authored percent, and the FIRST success ends the rolling by dropping a
// RANDOM piece of that set. So `pct` is the set's real per-kill drop chance,
// independent of how many pieces the set has (a per-piece roll would inflate it
// to 1-(1-pct)^n). Set membership comes from items.yaml `set:` keys.
func (cs *CombatSystem) rollChampionSetDrops(m *monster.Monster3D) {
	ladder := config.ChampionSetDrops()
	if len(ladder) == 0 {
		return
	}
	for _, drop := range ladder {
		if rand.Intn(100) >= drop.Pct {
			continue
		}
		keys := itemKeysOfSet(drop.Set)
		if len(keys) == 0 {
			continue // empty set (content edit): fall through to the next rung
		}
		it, err := items.TryCreateItemFromYAML(keys[rand.Intn(len(keys))])
		if err != nil {
			continue
		}
		cs.game.AddColoredCombatMessage(fmt.Sprintf("%s's %s drops on the sand!", m.Name, it.Name), combatMessageGold)
		cs.game.addLootBagDrop(m.X, m.Y, []items.Item{it}, 0)
		return // first successful set ends the rolling: one piece max per kill
	}
}

// itemKeysOfSet lists items.yaml keys carrying the given set key, sorted.
func itemKeysOfSet(set string) []string {
	if config.GlobalItems == nil {
		return nil
	}
	var keys []string
	for key, def := range config.GlobalItems.Items {
		if def != nil && def.Set == set {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// arenaTierSpentToday reports whether the difficulty was already challenged
// this arena day (the lockout expires at sunrise).
func (g *MMGame) arenaTierSpentToday(tierName string) bool {
	day, fought := g.arenaTierFoughtDay[tierName]
	return fought && day == g.dayNightDay
}

// dialogueChoiceLabel decorates a choice's display text with live state: a
// duel tier on its daily cooldown is marked so the player sees it before
// picking. Every choice renderer reads labels through this one helper.
func (g *MMGame) dialogueChoiceLabel(choice *character.NPCDialogueChoice) string {
	if choice == nil {
		return ""
	}
	if choice.Action == "start_arena_duel" && g.arenaTierSpentToday(choice.Tier) {
		if g.dayNightIsNight {
			return choice.Text + " - spent, returns at dawn"
		}
		return choice.Text + " - spent, returns at dusk"
	}
	return choice.Text
}

// ValidateDuelGrounds fails fast when any map places an NPC offering
// start_arena_duel without that map authoring a map_configs `duel:` block -
// otherwise the choice is a permanently dead button discovered only in play.
// Runs after LoadAllMaps (boot can't see NPC placements).
func ValidateDuelGrounds(wm *world.WorldManager) error {
	if wm == nil {
		return nil
	}
	for mapKey, w := range wm.LoadedMaps {
		for _, npc := range w.NPCs {
			if npcDialogueHasAction(npc, "start_arena_duel") {
				if mc := wm.MapConfigs[mapKey]; mc == nil || mc.Duel == nil {
					return fmt.Errorf("map %q places duel NPC %q but authors no duel: block in map_configs.yaml", mapKey, npc.Name)
				}
			}
		}
	}
	return nil
}

// rollChampionMonsterKey picks a RANDOM champion from the registry and returns
// the monster key that carries that build (monsters.yaml `champion:` link).
func rollChampionMonsterKey() string {
	keys := config.ChampionKeys() // sorted: the random index is reproducible
	if len(keys) == 0 || monster.MonsterConfig == nil {
		return ""
	}
	champion := keys[rand.Intn(len(keys))]
	// Deterministic mob resolution: scan monster keys in sorted order so two
	// mobs sharing one champion build always resolve to the same one.
	mKeys := make([]string, 0, len(monster.MonsterConfig.Monsters))
	for k := range monster.MonsterConfig.Monsters {
		mKeys = append(mKeys, k)
	}
	sort.Strings(mKeys)
	for _, mKey := range mKeys {
		if monster.MonsterConfig.Monsters[mKey].Champion == champion {
			return mKey
		}
	}
	return ""
}

// startArenaDuel stages a champion duel from a dialogue choice: the difficulty
// tier comes from the choice, the champion is rolled randomly from the
// registry, the party is placed on the map's duel ground (map_configs `duel`
// block) and the door reconciler bars the gates on the next frame. Each tier
// can be challenged once per day - starting the duel spends the attempt; the
// lockout expires at the next sunrise.
func (ih *InputHandler) startArenaDuel(choice *character.NPCDialogueChoice) {
	g := ih.game
	g.dialogActive = false
	g.dialogNPC = nil

	if g.livingChampion() != nil {
		g.AddCombatMessage("A duel is already underway - finish it first!")
		return
	}
	tier := config.GetChampionTier(choice.Tier)
	if choice.Tier == "" || tier == nil {
		g.AddCombatMessage("There is no duelling ground here.")
		return
	}
	if g.arenaTierSpentToday(choice.Tier) {
		g.AddCombatMessage("That challenge is spent for today - return after sunrise.")
		return
	}

	wm := world.GlobalWorldManager
	if wm == nil {
		g.AddCombatMessage("There is no duelling ground here.")
		return
	}
	mc := wm.MapConfigs[wm.CurrentMapKey]
	monsterKey := rollChampionMonsterKey()
	if mc == nil || mc.Duel == nil || monsterKey == "" {
		// Content bug (champion builds are validated at boot; the duel block is
		// per-map and only checkable here) - surface it instead of a silent no-op.
		g.AddCombatMessage("There is no duelling ground here.")
		return
	}

	if g.arenaTierFoughtDay == nil {
		g.arenaTierFoughtDay = make(map[string]int)
	}
	g.arenaTierFoughtDay[choice.Tier] = g.dayNightDay

	ts := float64(g.config.GetTileSize())
	px, py := TileCenterFromTile(mc.Duel.PartyTile[0], mc.Duel.PartyTile[1], ts)
	g.camera.X, g.camera.Y = px, py
	g.snapFacing(mc.Duel.FacingDeg * math.Pi / 180)
	if g.collisionSystem != nil {
		g.collisionSystem.UpdateEntity("player", px, py)
	}

	cx, cy := TileCenterFromTile(mc.Duel.ChampionTile[0], mc.Duel.ChampionTile[1], ts)
	m := monster.NewMonster3DFromConfig(cx, cy, monsterKey, g.config)
	m.ChampionTier = choice.Tier
	// Mirror NOW, not on the next monster update: player input runs before it,
	// and a first-frame hit on a still-placeholder mob (yaml HP/armor) would be
	// swallowed by the tier HP clamp.
	g.mirrorChampionStats(m)
	m.WasAttacked = true // engage immediately: the duel starts now
	m.IsEngagingPlayer = true
	g.registerSpawnedMonster(m)
	g.AddCombatMessage(fmt.Sprintf("%s (%s) steps onto the sand. The portcullises slam down!", m.Name, choice.Tier))
}

// championSpellPool resolves a champion's castable pool: every non-utility
// (combat) spell of its authored spell_schools plus the explicitly authored
// extra_spells (which may be utility-effect spells like the earth Stun).
// Deterministic order - schools as authored, spells sorted by the catalog.
func championSpellPool(def *config.ChampionDefinition) []spells.SpellID {
	var pool []spells.SpellID
	seen := map[spells.SpellID]bool{}
	add := func(id spells.SpellID) {
		if !seen[id] {
			seen[id] = true
			pool = append(pool, id)
		}
	}
	for _, school := range def.SpellSchools {
		for _, key := range config.GetSpellsBySchool(school) {
			// Combat spells only: no utility, and no behavior-effect projectiles
			// (deals_no_damage - Charm's pacify / Disintegrate's instakill have no
			// party-side meaning and would land as bare damage-less hits).
			if sd, ok := config.GetSpellDefinition(key); ok && sd != nil && !sd.IsUtility && !sd.DealsNoDamage {
				add(spells.SpellID(key))
			}
		}
	}
	for _, key := range def.ExtraSpells {
		add(spells.SpellID(key))
	}
	return pool
}

// championOpensWith reports whether the champion's authored opening spell
// applies at this tier (empty tier list = every tier).
func championOpensWith(def *config.ChampionDefinition, tierName string) bool {
	if def.OpeningSpell == "" {
		return false
	}
	if len(def.OpeningSpellTiers) == 0 {
		return true
	}
	for _, t := range def.OpeningSpellTiers {
		if t == tierName {
			return true
		}
	}
	return false
}

// validateChampionSpells fail-fasts the spellcasting block of one build:
// every named spell must exist, and a cast chance needs a non-empty pool.
// School keys are validated by BuildChampion. Called from PrimeChampions.
func validateChampionSpells(key string, def *config.ChampionDefinition) error {
	for _, sk := range def.ExtraSpells {
		if _, err := spells.GetSpellDefinitionByID(spells.SpellID(sk)); err != nil {
			return fmt.Errorf("champion %q: unknown extra spell %q", key, sk)
		}
	}
	if def.OpeningSpell != "" {
		if _, err := spells.GetSpellDefinitionByID(spells.SpellID(def.OpeningSpell)); err != nil {
			return fmt.Errorf("champion %q: unknown opening spell %q", key, def.OpeningSpell)
		}
		for _, t := range def.OpeningSpellTiers {
			if config.GetChampionTier(t) == nil {
				return fmt.Errorf("champion %q: opening_spell_tiers names unknown tier %q", key, t)
			}
		}
	}
	if def.SpellCastChance > 0 && len(championSpellPool(def)) == 0 {
		return fmt.Errorf("champion %q: spell_cast_chance authored but the spell pool is empty", key)
	}
	return nil
}

// championTryCastSpell is the cast-instead-of-attack gate, run at every
// champion attack moment (the RT attack tick and each TB swing both funnel
// through the shared attack entries). The authored opening spell always
// claims the FIRST action of the duel; afterwards each attack rolls
// spell_cast_chance to become a random pool cast. Returns whether a cast
// consumed the attack.
func (cs *CombatSystem) championTryCastSpell(m *monster.Monster3D) bool {
	if m == nil || !m.IsChampion() {
		return false
	}
	def := config.GetChampionDefinition(m.ChampionKey)
	ch := cs.game.championTemplateFor(m)
	if def == nil || ch == nil {
		return false
	}
	if !m.OpeningSpellDone && championOpensWith(def, championTierOf(m)) {
		m.OpeningSpellDone = true
		cs.championCastSpell(m, ch, spells.SpellID(def.OpeningSpell))
		return true
	}
	if def.SpellCastChance <= 0 || rand.Float64() >= def.SpellCastChance {
		return false
	}
	pool := championSpellPool(def)
	if len(pool) == 0 {
		return false
	}
	cs.championCastSpell(m, ch, pool[rand.Intn(len(pool))])
	return true
}

// championCastSpell resolves ONE champion cast through the real player spell
// pipeline - the caster is the champion's character template, so effective
// stats, school mastery and Luck crit all apply - dispatched by the spell's
// mechanical effect the same way the party cast path dispatches:
//   - Stone Skin family (incoming_damage_reduction): mastery-scaled flat soak
//     on the mob for the spell's duration (the monster dual of the party buff).
//   - AoE stun family (stun_radius_tiles): stuns every party member while the
//     party stands inside the shockwave radius.
//   - everything else: a real spell projectile at the party; AoE spells engulf
//     the whole party at impact, stun riders re-stamp at impact.
func (cs *CombatSystem) championCastSpell(m *monster.Monster3D, ch *character.MMCharacter, spellID spells.SpellID) {
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil {
		return
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s casts %s!", m.Name, def.Name))

	switch {
	case def.IncomingDamageReduction > 0:
		m.SoakDamage = scaledIncomingDamageReduction(def, ch)
		m.SoakFrames = cs.CalculateSpellDurationFrames(spellID, ch)
		// Stun convention: 1s per TB turn, floored at 1 whenever the soak is
		// active (a sub-1s duration would truncate to 0 turns and, since TB never
		// ticks SoakFrames, never expire in turn-based play).
		m.SoakTurns = m.SoakFrames / cs.game.config.GetTPS()
		if m.SoakTurns < 1 && m.SoakFrames > 0 {
			m.SoakTurns = 1
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s's skin hardens to stone!", m.Name))

	case def.StunRadiusTiles > 0:
		radius := def.StunRadiusTiles * float64(cs.game.config.GetTileSize())
		if Distance(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y) > radius {
			cs.game.AddCombatMessage("The shockwave dissipates short of the party.")
			return
		}
		frames := def.StunDurationSeconds * cs.game.config.GetTPS()
		stunned := 0
		cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
			cs.applyScaledCharStun(member, frames, def.StunDurationTurns)
			stunned++
		})
		cs.game.AddColoredCombatMessage(fmt.Sprintf("The shockwave stuns %d hero(es)!", stunned), combatMessageYellow)

	default:
		_, _, total := cs.CalculateSpellDamage(spellID, ch)
		total, _ = cs.rollSpellCritDamage(spellID, ch, total)
		cs.spawnMonsterSpellProjectileDamage(m, spellID, cs.game.camera.X, cs.game.camera.Y, ProjectileOwnerMonster, total)
	}
}

// stampChampionSpellRiders re-arms a champion's char-stun riders from the
// SPELL that actually flew (lightning, psychic_shock), at impact time - the
// spell dual of stampChampionProjectileRiders, because weapon bolts landing
// mid-flight re-stamp the same rider fields for their hand.
func (cs *CombatSystem) stampChampionSpellRiders(src *monster.Monster3D, spellType string) {
	if src == nil || !src.IsChampion() || spellType == "" {
		return
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellType))
	if err != nil {
		return
	}
	src.StunCharChance = def.StunChance
	src.StunCharSeconds = def.StunDurationSeconds
	src.StunCharTurns = def.StunDurationTurns
}
