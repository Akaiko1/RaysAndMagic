//go:build balance

package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// balanceArena runs bench-party fights through the real turn-based loop on
// the arena's sand hall. The party plays like a reasonable player: the active
// hero clicks the nearest enemy (the smart attack heals, casts the readied
// spell, arms a trap or swings), heroes with nothing in reach let the others
// act, and a party with nothing to do steps toward the enemy.
type balanceArena struct {
	t     *testing.T
	g     *MMGame
	bench *BenchCatalog
	ts    float64
	wm    *world.WorldManager
	// noSummons drops the pre-fight summons, to measure what they add.
	noSummons   bool
	actions     map[string]map[string]int // the running fight's action tally
	techSeen    map[string]int            // techniques already noted this fight
	onFrame     func()                    // optional probe, called after every frame
	readyFrames int                       // consecutive frames the party could have acted
}

// Sand-hall tiles: the party stands at the west edge facing east, the foes
// line up five to six tiles away.
var (
	balancePartyTile = [2]int{11, 5}
	balanceFoeTiles  = [][2]int{{16, 4}, {17, 5}, {16, 6}, {17, 3}, {17, 7}, {18, 4}, {18, 6}, {18, 5}}
)

const (
	balanceRoundCap = 60
	balanceFrameCap = balanceRoundCap * 600
)

func newBalanceArena(t *testing.T) *balanceArena {
	t.Helper()
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	bench, err := LoadBenchCatalog("assets/bench_parties.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.switchToMap("arena"); err != nil {
		t.Fatal(err)
	}
	return &balanceArena{t: t, g: g, bench: bench, ts: float64(cfg.GetTileSize()), wm: wm}
}

type balanceFight struct {
	Won, Timeout bool
	Rounds       int
	HPLostPct    float64 // party HP lost over party max HP, downed heroes count in full
	SPSpentPct   float64
	KOs          int     // heroes down at the end
	Potions      int     // consumables used
	MaxHitPct    float64 // biggest single hit on a hero, % of that hero's max HP
	OneShots     int     // hits that took a hero from 80%+ HP straight to 0
	BigHitBy     string  // who landed MaxHitPct

	Log     []string                  // every combat-log line of the fight
	Actions map[string]map[string]int // hero name -> action kind -> count
	Enraged map[string]bool           // foe keys that enraged
	Foes    map[string]balanceFoeRef  // foe monster ID -> key and display name
	Heroes  map[string]string         // hero name -> class key
}

// balanceFoeRef: the log names a monster by display name, which several keys
// can share (dragon_gold and dragon_green are both "Dragon").
type balanceFoeRef struct{ Key, Name string }

// noteHit records one hostile hit on a hero, given the hero's HP just before it.
func (r *balanceFight) noteHit(source string, damage, before, maxHP int) {
	if maxHP <= 0 {
		return
	}
	if pct := 100 * float64(damage) / float64(maxHP); pct > r.MaxHitPct {
		r.MaxHitPct, r.BigHitBy = pct, source
	}
	if damage >= before && 10*before >= 8*maxHP {
		r.OneShots++
	}
}

// captureLog appends the combat-log lines written since version.
func (a *balanceArena) captureLog(r *balanceFight, version int) int {
	g := a.g
	fresh := g.combatLogVersion - version
	history := g.combatLogHistory
	for _, e := range history[max(0, len(history)-fresh):] {
		r.Log = append(r.Log, e.Text)
	}
	return g.combatLogVersion
}

// stage sets up one encounter: a fresh party, prepared, facing its foes at
// the start of a new turn-based fight.
func (a *balanceArena) stage(partyKey string, level int, foes []string) {
	a.t.Helper()
	g := a.g
	a.reset()
	p := a.bench.Parties[partyKey]
	members, err := g.buildBenchMembers(p, level)
	if err != nil {
		a.t.Fatal(err)
	}
	g.party = character.NewPartyFromGroups(g.config, members, nil, nil)
	g.selectedChar = 0
	if err := g.applyBenchPartyState(p, level); err != nil {
		a.t.Fatal(err)
	}
	px, py := TileCenterFromTile(balancePartyTile[0], balancePartyTile[1], a.ts)
	g.setPartyPosition(px, py)
	g.snapFacing(0)
	a.prepareParty()
	for i, key := range foes {
		tile := balanceFoeTiles[i%len(balanceFoeTiles)]
		x, y := TileCenterFromTile(tile[0], tile[1], a.ts)
		m := monster.NewMonster3DFromConfig(x, y, key, g.config)
		m.Experience = 0 // no level-up mid-fight
		a.unseal(m)
		g.registerSpawnedMonster(m)
	}
	g.ToggleTurnBasedMode()
}

// fight stages one encounter and runs it to a decision or the round cap.
func (a *balanceArena) fight(partyKey string, level int, foes []string) balanceFight {
	a.t.Helper()
	g := a.g
	a.stage(partyKey, level, foes)
	startHP, maxHP, maxSP, consumables := 0, 0, 0, a.consumableCount()
	for _, m := range g.party.Members {
		startHP += m.HitPoints
		maxHP += m.MaxHitPoints
		maxSP += m.MaxSpellPoints
	}
	result := balanceFight{Actions: map[string]map[string]int{}, Enraged: map[string]bool{}, Foes: map[string]balanceFoeRef{}, Heroes: map[string]string{}}
	for _, m := range g.party.Members {
		result.Heroes[m.Name] = m.Class.Key()
	}
	logVersion := g.combatLogVersion
	g.heroHitObserver = func(hero *character.MMCharacter, source string, damage int) {
		result.noteHit(source, damage, hero.HitPoints, hero.MaxHitPoints)
	}
	defer func() { g.heroHitObserver = nil }()
	a.actions = result.Actions
	a.techSeen = map[string]int{}
	monsterTurn := false
	for frame := 0; frame < balanceFrameCap; frame++ {
		if !a.foesStanding() {
			result.Won = true
			break
		}
		if a.partyDown() {
			break
		}
		if result.Rounds >= balanceRoundCap {
			result.Timeout = true
			break
		}
		// The party decides after the frame, as input from a player lands
		// between frames: autocast techniques get their window first. A
		// monster turn can open and resolve inside one frame, so the turn is
		// observed after the decision and again after the frame.
		g.gameLoop.updateExploration()
		g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
		logVersion = a.captureLog(&result, logVersion)
		a.noteTechniques()
		if a.onFrame != nil {
			a.onFrame()
		}
		for _, m := range g.world.Monsters {
			if balanceFoe(m) {
				result.Foes[m.ID] = balanceFoeRef{Key: m.Key, Name: m.Name}
				if m.IsEnraged() {
					result.Enraged[m.Key] = true
				}
			}
		}
		if monsterTurn && g.currentTurn == 0 {
			result.Rounds++
			monsterTurn = false
		}
		monsterTurn = monsterTurn || g.currentTurn == 1
		a.partyAct()
		monsterTurn = monsterTurn || g.currentTurn == 1
	}
	if !result.Won && !a.partyDown() && result.Rounds < balanceRoundCap {
		result.Timeout = true
	}
	a.captureLog(&result, logVersion)
	hp, sp := 0, 0
	for _, m := range g.party.Members {
		if balanceHeroDown(m) {
			result.KOs++
			continue
		}
		hp += m.HitPoints
		sp += m.SpellPoints
	}
	result.HPLostPct = 100 * float64(max(0, startHP-hp)) / float64(max(1, maxHP))
	result.SPSpentPct = 100 * float64(maxSP-sp) / float64(max(1, maxSP))
	result.Potions = consumables - a.consumableCount()
	return result
}

// prepareParty does what a player does before a fight: switch on the
// automatic techniques, call summoned allies and cast every known buff that
// changes a fight (stats, resistances, damage dealt or taken), through the
// real cast path.
func (a *balanceArena) prepareParty() {
	g := a.g
	for _, m := range g.party.Members {
		if m.Class == character.ClassWayfarer && config.GlobalTechniques != nil {
			if m.RareClass.Automatic == nil {
				m.RareClass.Automatic = map[string]bool{}
			}
			for _, d := range config.GlobalTechniques.Techniques {
				if d.Automatic {
					m.RareClass.Automatic[d.Key] = true
				}
			}
		}
		for _, school := range m.MagicSchools {
			for _, id := range school.KnownSpells {
				def, err := spells.GetSpellDefinitionByID(id)
				fightBuff := def.IsBuff() && (def.StatBonus != 0 || len(def.StatBonuses) > 0 || def.ResistBuffPct != 0 ||
					def.ResistBuffSchoolPct != 0 || def.OutgoingDamageBonus != 0 || def.IncomingDamageReduction != 0)
				if err != nil || !(fightBuff || (def.SummonMonster != "" && !a.noSummons)) {
					continue
				}
				g.combat.castPlayerSpell(id, def, m, false)
			}
		}
	}
	// Buffed out of combat, then rested: buffs that raise max HP or SP leave
	// the party full, not short of its new maximum.
	for _, m := range g.party.Members {
		m.HitPoints, m.SpellPoints = m.MaxHitPoints, m.MaxSpellPoints
	}
}

// unseal puts a quest-gated boss in the state the player meets it: its gating
// quest done. Idol wards drop on their own, as the arena has no idols.
func (a *balanceArena) unseal(m *monster.Monster3D) {
	if m.PassiveUntilQuest == "" || a.g.questManager == nil {
		return
	}
	if q := a.g.questManager.GetQuest(m.PassiveUntilQuest); q == nil || q.Status != quests.QuestStatusCompleted {
		_ = a.g.questManager.ActivateQuest(m.PassiveUntilQuest)
		a.g.questManager.MarkCompleted(m.PassiveUntilQuest)
	}
	m.BossDormant = false
}

// partyAct takes one party decision on a frame where the player could act.
func (a *balanceArena) partyAct() {
	g := a.g
	ih := g.gameLoop.inputHandler
	if !g.turnBasedMode || g.currentTurn != 0 || g.viewTurnFramesLeft > 0 || g.spellInputCooldown != 0 || g.partyAllExhausted() || g.gameplayPausedByOverlay() {
		a.readyFrames = 0
		return
	}
	// A player reacts a frame late, which leaves automatic techniques the
	// first free frame (they run before the cooldown ticks within a frame).
	if a.readyFrames++; a.readyFrames < 2 {
		return
	}
	a.readyFrames = 0
	target := a.nearestFoe()
	if target == nil {
		return
	}
	if a.revive() {
		return
	}
	// A mostly melee party closes in before acting: a step after an action
	// hands the monsters an extra turn.
	if g.partyActionsUsed == 0 && a.mostlyMelee() &&
		math.Hypot(target.X-g.camera.X, target.Y-g.camera.Y) > 1.5*a.ts && a.stepToward(target) {
		a.tally("party", "step")
		return
	}
	// The active hero first, then anyone else with an action left.
	order := []int{g.selectedChar}
	for i := range g.party.Members {
		if i != g.selectedChar {
			order = append(order, i)
		}
	}
	for _, i := range order {
		if !g.canSelectChar(i) {
			continue
		}
		g.selectedChar = i
		before := a.turnStamp()
		probe := a.actionProbe(i)
		ih.performMouseSmartAttack(target)
		if a.turnStamp() != before {
			a.tally(g.party.Members[i].Name, probe.kind(a, i))
			return
		}
	}
	// Nothing in reach: close the distance like the move keys do.
	if a.stepToward(target) {
		a.tally("party", "step")
		return
	}
	a.tally("party", "pass")
	g.forfeitPartyTurn()
}

// revive raises a downed hero the way a player does: a revive spell from a
// quick slot, through the quick-slot action.
func (a *balanceArena) revive() bool {
	g := a.g
	down := false
	for _, m := range g.party.Members {
		down = down || (balanceHeroDown(m) && !m.HasCondition(character.ConditionEradicated))
	}
	if !down {
		return false
	}
	for i, m := range g.party.Members {
		if !g.canSelectChar(i) {
			continue
		}
		for slot, q := range m.QuickSlots {
			if q == nil {
				continue
			}
			def, err := spells.GetSpellDefinitionByID(spells.SpellID(q.SpellEffect))
			if err != nil || !(def.Revive || def.ReviveHpPct > 0) || m.SpellPoints < g.combat.effectiveSpellCost(m, def.SpellPointsCost) {
				continue
			}
			g.selectedChar = i
			before := a.turnStamp()
			g.useQuickSlot(i, slot)
			if a.turnStamp() != before {
				a.tally(m.Name, "revive:"+string(q.SpellEffect))
				return true
			}
		}
	}
	return false
}

// noteTechniques records the autocast techniques in effect: they announce
// nothing, so the party's buffs and Purify's lock are what shows them.
func (a *balanceArena) noteTechniques() {
	g := a.g
	for _, m := range g.party.Members {
		if m.Class != character.ClassWayfarer {
			continue
		}
		for _, b := range g.combatBuffs {
			if config.Technique(b.SpellID) != nil && a.techSeen[m.Name+b.SpellID] == 0 {
				a.techSeen[m.Name+b.SpellID] = 1
				a.tally(m.Name, "technique:"+b.SpellID)
			}
		}
		if m.RareClass.PurifyFrames > 0 && a.techSeen[m.Name+"purify"] == 0 {
			a.techSeen[m.Name+"purify"] = 1
			a.tally(m.Name, "technique:purify")
		}
	}
}

func (a *balanceArena) tally(who, kind string) {
	if a.actions == nil {
		return
	}
	if a.actions[who] == nil {
		a.actions[who] = map[string]int{}
	}
	a.actions[who][kind]++
}

// actionProbe snapshots what one hero's action can change, so the action
// can be named afterwards from its effect.
type actionProbe struct {
	sp, traps, flasks, alliesHP int
	readied                     string
}

func (a *balanceArena) actionProbe(i int) actionProbe {
	g := a.g
	m := g.party.Members[i]
	p := actionProbe{sp: m.SpellPoints, traps: len(g.traps), readied: string(m.Equipment[items.SlotSpell].SpellEffect)}
	if m.Equipment[items.SlotSpell].Type == items.ItemThrowable {
		p.flasks = g.flaskStock(m, p.readied)
	}
	for _, o := range g.party.Members {
		p.alliesHP += o.HitPoints
	}
	return p
}

func (p actionProbe) kind(a *balanceArena, i int) string {
	g := a.g
	m := g.party.Members[i]
	hp := 0
	for _, o := range g.party.Members {
		hp += o.HitPoints
	}
	_, readiedTrap := config.TrapItem(p.readied)
	switch {
	case len(g.traps) > p.traps || (readiedTrap && m.SpellPoints < p.sp):
		return "trap"
	case m.Equipment[items.SlotSpell].Type == items.ItemThrowable && g.flaskStock(m, p.readied) < p.flasks:
		return "flask:" + p.readied
	case m.SpellPoints < p.sp && hp > p.alliesHP:
		return "heal"
	case m.SpellPoints < p.sp:
		return "spell:" + p.readied
	}
	return "weapon"
}

// stepToward moves the party one tile toward target with the move keys'
// rule: the step ends the party turn.
func (a *balanceArena) stepToward(target *monster.Monster3D) bool {
	g := a.g
	ih := g.gameLoop.inputHandler
	dx, dy := target.X-g.camera.X, target.Y-g.camera.Y
	sx, sy := 0, 0
	if math.Abs(dx) >= math.Abs(dy) {
		sx = int(math.Copysign(1, dx))
	} else {
		sy = int(math.Copysign(1, dy))
	}
	if !ih.moveTurnBasedInDirection(sx, sy) && !ih.moveTurnBasedInDirection(sy, sx) {
		return false
	}
	g.turnBasedMoveCooldown = int(TurnBasedInputCooldownSeconds * float64(g.config.GetTPS()))
	g.endPartyTurnAfterMovement()
	return true
}

// mostlyMelee: fewer than half the standing heroes can act from range (a
// ranged weapon, or a readied offensive spell, flask or trap).
func (a *balanceArena) mostlyMelee() bool {
	g := a.g
	ranged, standing := 0, 0
	for _, m := range g.party.Members {
		if balanceHeroDown(m) {
			continue
		}
		standing++
		action := m.Equipment[items.SlotSpell]
		_, spell := g.combat.smartOffensiveSpell(m)
		weapon := lookupWeaponConfigByName(m.Equipment[items.SlotMainHand].Name)
		if spell || action.Type == items.ItemThrowable || action.Type == items.ItemTrap || (weapon != nil && weapon.IsRanged()) {
			ranged++
		}
	}
	return 2*ranged < standing
}

type balanceTurnStamp struct {
	used, remaining, turn int
}

func (a *balanceArena) turnStamp() balanceTurnStamp {
	s := balanceTurnStamp{used: a.g.partyActionsUsed, turn: a.g.currentTurn}
	for _, m := range a.g.party.Members {
		s.remaining += m.ActionsRemaining
	}
	return s
}

func (a *balanceArena) nearestFoe() *monster.Monster3D {
	var best *monster.Monster3D
	bestD := math.MaxFloat64
	for _, m := range a.g.world.Monsters {
		if !balanceFoe(m) || !pointerAttackable(m) {
			continue
		}
		if d := math.Hypot(m.X-a.g.camera.X, m.Y-a.g.camera.Y); d < bestD {
			best, bestD = m, d
		}
	}
	return best
}

func (a *balanceArena) foesStanding() bool {
	for _, m := range a.g.world.Monsters {
		if balanceFoe(m) {
			return true
		}
	}
	return false
}

func balanceFoe(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !m.Bound
}

func (a *balanceArena) partyDown() bool {
	for _, m := range a.g.party.Members {
		if !balanceHeroDown(m) {
			return false
		}
	}
	return true
}

func balanceHeroDown(m *character.MMCharacter) bool {
	return m == nil || m.HitPoints <= 0 || m.HasCondition(character.ConditionUnconscious) ||
		m.HasCondition(character.ConditionDead) || m.HasCondition(character.ConditionEradicated)
}

func (a *balanceArena) consumableCount() int {
	n := 0
	count := func(it *items.Item) {
		if it != nil && it.Type == items.ItemConsumable {
			n += it.Count()
		}
	}
	for i := range a.g.party.Inventory {
		count(&a.g.party.Inventory[i])
	}
	for _, m := range a.g.party.Members {
		for _, q := range m.QuickSlots {
			count(q)
		}
	}
	return n
}

// reset clears everything a previous fight left on the map or the party, and
// leaves turn-based mode so the next fight opens a fresh turn.
func (a *balanceArena) reset() {
	g := a.g
	for _, m := range g.world.Monsters {
		g.collisionSystem.UnregisterEntity(m.ID)
	}
	g.world.Monsters = nil
	g.clearTransientCombatState()
	g.resetTimedEffects() // buffs, damage zones, root, brewing
	g.groundContainers = nil
	g.traps = nil
	for i := range g.cardSlots {
		g.clearCardCollectionSlot(i)
	}
	g.gameOver = false
	g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
	g.spellInputCooldown, g.viewTurnFramesLeft = 0, 0
	if g.turnBasedMode {
		g.ToggleTurnBasedMode()
	}
	// Leaving TB parks the turn for a return; entering it again would resume
	// the old fight's turn and spent actions instead of starting a new one.
	g.turnBasedTurnSuspended = false
}
