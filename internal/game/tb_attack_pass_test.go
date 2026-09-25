package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
)

const tbPassMessage = "Nothing left to attack with"

// tbAttackLoadouts are the SmartAttack branches, plus the loadouts with none.
// The holder is the only member with an action slot left.
var tbAttackLoadouts = []string{"unarmed", "spell without SP", "monk spell", "armed", "offensive spell", "heal plan", "trap"}

func equipTBAttackLoadout(t *testing.T, g *MMGame, idx int, loadout string) *character.MMCharacter {
	t.Helper()
	m := g.party.Members[idx]
	if loadout == "trap" {
		m = character.CreateCharacter("Trapper", character.ClassThief, g.config)
		g.party.Members[idx] = m
		trap, ok := config.TrapItem("cleave_trap")
		if !ok {
			t.Fatal("cleave_trap missing")
		}
		m.Equipment[items.SlotSpell] = trap
	}
	delete(m.Equipment, items.SlotMainHand)
	delete(m.Equipment, items.SlotOffHand)
	m.SpellPoints, m.MaxSpellPoints = 100, 100
	fireball := items.Item{Type: items.ItemBattleSpell, SpellEffect: "fireball", SpellCost: 4}
	switch loadout {
	case "unarmed":
		m.Equipment[items.SlotSpell] = items.Item{Name: "Fly", Type: items.ItemUtilitySpell, SpellEffect: "fly", SpellCost: 12}
	case "spell without SP":
		m.Equipment[items.SlotSpell] = fireball
		m.SpellPoints = 0
	case "monk spell":
		m.Class = character.ClassMonk
		m.Equipment[items.SlotSpell] = fireball
	case "armed":
		m.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	case "offensive spell":
		m.LearnSpell("fireball")
		m.Equipment[items.SlotSpell] = fireball
	case "heal plan":
		m.LearnSpell("heal_other")
		m.Equipment[items.SlotSpell] = items.Item{Name: "Heal", Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffectHealOther, SpellCost: 4}
		hurt := g.party.Members[(idx+1)%len(g.party.Members)]
		hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
	}
	m.HitPoints = m.MaxHitPoints
	return m
}

// strandTBParty leaves only the holder with an action slot.
func strandTBParty(g *MMGame, holder int) {
	for i, m := range g.party.Members {
		m.ActionsRemaining = 0
		if i == holder {
			m.ActionsRemaining = 1
		}
	}
	g.selectedChar = holder
	g.currentTurn = 0
}

func tbPassAnnounced(g *MMGame) bool {
	for _, msg := range g.GetCombatMessages() {
		if strings.Contains(msg, tbPassMessage) {
			return true
		}
	}
	return false
}

// Case table: entry point x the holder's loadout. An attack request that no
// slot holder can carry out ends the turn; any branch SmartAttack owns keeps
// it. F and C are not attack requests and never pass.
func TestTBAttackRequestPassesWhenNobodyCanAttack(t *testing.T) {
	const (
		pass   = "pass"
		act    = "act"
		keep   = "keep"
		noPass = "no pass"
	)
	want := map[string]map[string]string{
		"unarmed":          {"press": pass, "hold": pass, "space": pass, "R": pass, "F": noPass, "C": noPass},
		"spell without SP": {"press": pass, "hold": pass, "space": pass, "R": pass, "F": noPass, "C": noPass},
		"monk spell":       {"press": pass, "hold": pass, "space": pass, "R": pass, "F": noPass, "C": noPass},
		"armed":            {"press": act, "hold": act, "space": act, "R": act, "F": noPass, "C": noPass},
		"offensive spell":  {"press": act, "hold": act, "space": act, "R": keep, "F": noPass, "C": noPass},
		"heal plan":        {"press": act, "hold": act, "space": act, "R": keep, "F": noPass, "C": noPass},
		"trap":             {"press": act, "hold": act, "space": act, "R": keep, "F": noPass, "C": noPass},
	}
	keys := map[string]ebiten.Key{"space": ebiten.KeySpace, "R": ebiten.KeyR, "F": ebiten.KeyF, "C": ebiten.KeyC}
	for _, loadout := range tbAttackLoadouts {
		for _, entry := range []string{"press", "hold", "space", "R", "F", "C"} {
			t.Run(loadout+"/"+entry, func(t *testing.T) {
				g, ih, fp, m, tick := mouseCombatHarness(t, true)
				g.maxMessages = 50
				holder := equipTBAttackLoadout(t, g, 3, loadout)
				strandTBParty(g, 3)
				hp := m.HitPoints
				switch entry {
				case "press":
					fp.press()
					tick()
				case "hold":
					// The press is refused by the shared stagger; the repeat acts.
					g.spellInputCooldown = 30
					fp.press()
					tick()
					fp.hold()
					for i := 0; i < rtHoldRepeatDelay+1 && g.currentTurn == 0 && holder.ActionsRemaining > 0; i++ {
						tick()
					}
				default:
					fp.moveTo(5, 5)
					pressed := true
					key := keys[entry]
					ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed && k == key })
					tick()
					pressed = false
				}
				announced := tbPassAnnounced(g)
				switch want[loadout][entry] {
				case pass:
					if !announced || g.currentTurn != 1 || holder.ActionsRemaining != 0 || m.HitPoints != hp {
						t.Fatalf("want pass: announced=%v turn=%d slots=%d damaged=%v", announced, g.currentTurn, holder.ActionsRemaining, m.HitPoints != hp)
					}
				case act:
					if announced || holder.ActionsRemaining != 0 {
						t.Fatalf("want the holder to act: announced=%v slots=%d", announced, holder.ActionsRemaining)
					}
				case keep:
					if announced || g.currentTurn != 0 || holder.ActionsRemaining != 1 {
						t.Fatalf("want the slot kept: announced=%v turn=%d slots=%d", announced, g.currentTurn, holder.ActionsRemaining)
					}
				case noPass:
					if announced {
						t.Fatal("a non-attack request passed the turn")
					}
				}
			})
		}
	}
}

// The chain skips a slot holder who cannot attack, then the next request with
// only that holder left passes. Real time has no pass: the chain just moves on.
func TestPartyActionChainSkipsMembersWhoCannotAttack(t *testing.T) {
	t.Run("TB", func(t *testing.T) {
		g, _, fp, m, tick := mouseCombatHarness(t, true)
		g.maxMessages = 50
		idle := equipTBAttackLoadout(t, g, 3, "unarmed")
		strandTBParty(g, 3)
		g.party.Members[0].ActionsRemaining = 1
		hp := m.HitPoints
		fp.press()
		tick()
		if m.HitPoints == hp || g.party.Members[0].ActionsRemaining != 0 || idle.ActionsRemaining != 1 || tbPassAnnounced(g) {
			t.Fatalf("armed member did not take the request: damaged=%v slots=%d/%d", m.HitPoints != hp, g.party.Members[0].ActionsRemaining, idle.ActionsRemaining)
		}
		fp.release()
		tick()
		g.spellInputCooldown = 0
		fp.press()
		tick()
		if !tbPassAnnounced(g) || g.currentTurn != 1 {
			t.Fatalf("second request did not pass: turn=%d", g.currentTurn)
		}
	})
	for _, armedReady := range []bool{true, false} {
		t.Run(fmt.Sprintf("RT/armedReady=%v", armedReady), func(t *testing.T) {
			g, _, fp, m, tick := mouseCombatHarness(t, false)
			g.maxMessages = 50
			for i := range g.party.Members {
				if i != 0 || !armedReady {
					equipTBAttackLoadout(t, g, i, "unarmed")
				}
			}
			g.selectedChar = 3
			hp := m.HitPoints
			fp.press()
			tick()
			if armedReady != (m.HitPoints < hp) || tbPassAnnounced(g) {
				t.Fatalf("first press damaged=%v, want %v", m.HitPoints < hp, armedReady)
			}
		})
	}
}

// The stranded round survives save/load and still resolves from Space.
func TestTBAttackPassAfterSaveLoad(t *testing.T) {
	g, wm, _ := travelFixture(t)
	g.maxMessages = 50
	g.turnBasedMode = true
	equipTBAttackLoadout(t, g, 3, "unarmed")
	for i := 0; i < 3; i++ {
		g.party.Members[i].Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	}
	strandTBParty(g, 3)
	save := g.buildSave(wm)
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	var loaded GameSave
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := g.applySave(wm, &loaded); err != nil {
		t.Fatal(err)
	}
	if g.currentTurn != 0 || g.party.Members[3].ActionsRemaining != 1 || g.partyAllExhausted() {
		t.Fatalf("stranded round not restored: turn=%d slots=%d", g.currentTurn, g.party.Members[3].ActionsRemaining)
	}
	ih := NewInputHandler(g)
	ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeySpace })
	ih.keys.BeginFrame()
	g.spellInputCooldown = 0
	ih.handleTurnBasedInput()
	if !tbPassAnnounced(g) || g.currentTurn != 1 {
		t.Fatalf("restored round did not pass: turn=%d", g.currentTurn)
	}
}

// After the pass the monsters really take their turn: a stunned one spends a
// stunned turn and the party gets a fresh round.
func TestTBAttackPassRunsTheMonsterTurn(t *testing.T) {
	g, _, fp, m, tick := mouseCombatHarness(t, true)
	g.maxMessages = 50
	equipTBAttackLoadout(t, g, 3, "unarmed")
	strandTBParty(g, 3)
	g.combat.applyStunDR(m, 2, 2*g.config.GetTPS(), false)
	stunned := m.StunTurnsRemaining
	fp.press()
	tick()
	if g.currentTurn != 1 {
		t.Fatalf("turn = %d, want the monster turn", g.currentTurn)
	}
	g.gameLoop.updateMonstersTurnBased()
	if m.StunTurnsRemaining != stunned-1 || g.currentTurn != 0 || g.partyAllExhausted() {
		t.Fatalf("monster turn: stun %d -> %d, turn=%d exhausted=%v", stunned, m.StunTurnsRemaining, g.currentTurn, g.partyAllExhausted())
	}
}
