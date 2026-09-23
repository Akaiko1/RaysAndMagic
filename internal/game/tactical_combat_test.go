package game

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

func sniperFixture(t *testing.T, tb bool) (*MMGame, *GameLoop, *character.MMCharacter, float64) {
	t.Helper()
	g, gl, tile := tbBehaviorGame(t, 24, 24)
	g.turnBasedMode = tb
	placePlayerAtTile(g, 5, 10, tile)
	ch := character.CreateCharacter("Mara", character.ClassSniper, g.config)
	g.party.Members = []*character.MMCharacter{ch}
	g.party.Inventory = nil
	g.selectedChar = 0
	g.updateTacticalClocks()
	g.tactics.stationarySeconds = character.OverwatchReadySeconds
	g.tactics.movedTB = false
	g.combat.reactionRoll = func() float64 { return 0 }
	return g, gl, ch, tile
}

func TestOverwatchReadinessAndIndicatorContract(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, name := range []string{"ready", "turn", "step", "waiting", "cooldown", "stun", "dead", "no skill", "melee", "bow"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, name), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, tb)
				want := true
				switch name {
				case "turn":
					g.camera.Angle += math.Pi / 2
				case "step":
					g.camera.X += tile
					g.updateTacticalClocks()
					want = false
				case "waiting":
					g.tactics.stationarySeconds = 0
					g.tactics.movedTB = true
					want = false
				case "cooldown":
					ch.RTCooldown = 999
					ch.ActionsRemaining = 0
				case "stun":
					ch.StunFramesRemaining = 120
					want = false
				case "dead":
					ch.HitPoints = 0
					want = false
				case "no skill":
					delete(ch.Skills, character.SkillOverwatch)
					want = false
				case "melee":
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
					want = false
				case "bow":
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
				}
				if got := g.overwatchReady(ch); got != want {
					t.Fatalf("ready=%v want %v", got, want)
				}
			})
		}
	}
	g, _, ch, tile := sniperFixture(t, false)
	g.camera.X += tile
	g.updateTacticalClocks()
	for range g.config.GetTPS() - 1 {
		g.updateTacticalClocks()
	}
	if g.overwatchReady(ch) {
		t.Fatal("armed before a full stationary second")
	}
	g.updateTacticalClocks()
	if !g.overwatchReady(ch) {
		t.Fatal("not armed after a full stationary second")
	}
	g.ToggleTurnBasedMode()
	if g.overwatchReady(ch) {
		t.Fatal("mode switch granted readiness")
	}
	g.updateTacticalClocks()
	g.startPartyTurn()
	if !g.overwatchReady(ch) {
		t.Fatal("next round did not rearm stationary sniper")
	}
}

func TestOverwatchMovementCases(t *testing.T) {
	for _, name := range []string{"approach", "away", "stationary", "teleport", "bound", "pacified", "dead", "range", "wall", "partial", "failed roll"} {
		t.Run(name, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
			m.BeginPlayerEngagement()
			m.State = monster.StatePursuing
			ch.RTCooldown, ch.OffHandRTCooldown, ch.ActionsRemaining = 97, 81, 0
			oldX, oldY := m.X, m.Y
			m.X -= tile
			want := 1
			switch name {
			case "away":
				m.X = oldX + tile
				want = 0
			case "stationary":
				m.X = oldX
				want = 0
			case "teleport":
				m.X = oldX - 3*tile
				want = 0
			case "bound":
				m.Bound = true
				want = 0
			case "pacified":
				m.Pacified = true
				want = 0
			case "dead":
				m.HitPoints = 0
				want = 0
			case "range":
				oldX = 22.5 * tile
				m.X = oldX - tile
				want = 0
			case "wall":
				g.world.Tiles[10][7] = world.TileWall
				want = 0
			case "partial":
				m.X = oldX - tile/4
				want = 0
			case "failed roll":
				g.combat.reactionRoll = func() float64 { return 0.99 }
				want = 0
			}
			g.observeOverwatchMovement(m, oldX, oldY)
			if len(g.arrows) != want {
				t.Fatalf("shots=%d want %d", len(g.arrows), want)
			}
			if ch.RTCooldown != 97 || ch.OffHandRTCooldown != 81 || ch.ActionsRemaining != 0 {
				t.Fatal("reaction spent cooldown/actions")
			}
			g.observeOverwatchMovement(m, m.X, m.Y)
			if len(g.arrows) != want {
				t.Fatal("stationary update repeated reaction")
			}
			if want > 0 && (!g.arrows[0].WorldAim || g.arrows[0].Attacker != ch) {
				t.Fatal("reaction lost world aim or shooter")
			}
		})
	}
}

func TestOverwatchTurnBasedCommitAndRoundWiring(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, true)
	m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
	m.BeginPlayerEngagement()
	m.State = monster.StatePursuing
	ch.ActionsRemaining, ch.RTCooldown = 0, 91
	if !gl.commitMonsterMoveTB(m, m.X-tile, m.Y) || len(g.arrows) != 1 {
		t.Fatal("TB movement did not fire")
	}
	g.camera.Y += tile
	g.updateTacticalClocks()
	gl.commitMonsterMoveTB(m, m.X-tile, m.Y)
	if len(g.arrows) != 1 {
		t.Fatal("TB step did not block reaction")
	}
	g.startPartyTurn()
	beforeActions := ch.ActionsRemaining
	gl.commitMonsterMoveTB(m, m.X-tile, m.Y)
	if len(g.arrows) != 2 || ch.ActionsRemaining != beforeActions || ch.RTCooldown != 91 {
		t.Fatal("new round reaction failed or spent normal attack")
	}
}

func TestOverwatchRealTimeSchedulerWiring(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, false)
	g.threading = threading.NewThreadingComponents(g.config)
	defer g.threading.Shutdown()
	m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
	m.BeginPlayerEngagement()
	m.State = monster.StatePursuing
	ch.RTCooldown = 999
	for n := 0; n < g.config.GetTPS()*8 && len(g.arrows) == 0; n++ {
		g.frameCount++
		gl.runMonsterFrame()
	}
	if len(g.arrows) == 0 {
		t.Fatalf("serial movement publication never fired Overwatch (x=%.1f)", m.X/tile)
	}
	if ch.RTCooldown != 999 {
		t.Fatal("scheduler reaction spent cooldown")
	}
}

func TestOverwatchNormalShotPayloadParity(t *testing.T) {
	for _, key := range []string{"surveyors_rifle", "hunting_bow", "alien_blaster", "arbalest"} {
		t.Run(key, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			def, _, _ := config.GetWeaponDefinitionByName(items.CreateWeaponFromYAML(key).Name)
			def.CritChance = 100
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
			if !g.combat.EquipmentMeleeAttack() {
				t.Fatal("normal shot failed")
			}
			normal := g.arrows[len(g.arrows)-1]
			m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
			if !g.fireOverwatch(0, m) {
				t.Fatal("reaction shot failed")
			}
			a := g.arrows[len(g.arrows)-1]
			if a.Damage != normal.Damage || a.TrueDamage != normal.TrueDamage || a.DamageType != normal.DamageType || a.Crit != normal.Crit || a.PierceLeft != normal.PierceLeft || a.DisintegrateChance != normal.DisintegrateChance || a.LifeTime != normal.LifeTime {
				t.Fatalf("reaction payload drift: %+v vs %+v", a, normal)
			}
		})
	}
}

func TestTacticalSkillDataAndSaveContract(t *testing.T) {
	g, _, ch, _ := sniperFixture(t, false)
	if ch.HasSkill(character.SkillDagger) || ch.HasSkill(character.SkillBodybuilding) {
		t.Fatal("unwanted starting skills")
	}
	if g.config.Characters.Classes["sniper"].CardRarity != "legendary" {
		t.Fatal("sniper rarity")
	}
	ch.AutoDrinkCooldown = 37
	ch.DesignatedTargetID = "target"
	ch.DesignationFrames = 99
	saved := buildCharacterSave(ch)
	restored := restoreCharacterSave(saved)
	if restored.Class != ch.Class || restored.AutoDrinkCooldown != 37 || restored.DesignatedTargetID != "target" || restored.DesignationFrames != 99 || !restored.HasSkill(character.SkillOverwatch) {
		t.Fatal("tactical state lost on save round trip")
	}
	for _, skill := range []character.SkillType{character.SkillOverwatch, character.SkillBallistics, character.SkillFieldMedicine, character.SkillDesignateTarget} {
		text := skill.Description()
		if text == "" || strings.Contains(text, "%!") {
			t.Fatalf("invalid shared description: %s", text)
		}
		for _, r := range text {
			if r > 127 {
				t.Fatal("non-ASCII skill description")
			}
		}
	}
	for i := range g.party.Reserve {
		if g.party.Reserve[i].Name == "Mara" {
			g.party.Reserve = append(g.party.Reserve[:i], g.party.Reserve[i+1:]...)
			break
		}
	}
	g.party.Members = nil
	g.ensureAdditionalRecruits()
	n := len(g.party.Reserve)
	g.ensureAdditionalRecruits()
	if len(g.party.Reserve) != n || n == 0 || g.party.Reserve[n-1].Name != "Mara" {
		t.Fatal("recruit migration missing or duplicated")
	}
}

func TestDesignationUsesWeaponImpactAndExpires(t *testing.T) {
	g, _, ch, tile := sniperFixture(t, false)
	def, _ := config.GetWeaponDefinition("surveyors_rifle")
	def.CritChance = 0
	delete(ch.Skills, character.SkillBallistics)
	ch.Luck = 0
	g.combat.designationRoll = func(int) int { return 0 }
	m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
	m.HitPoints = 10000
	m.ArmorClass = 0
	m.PerfectDodge = 0
	m.Resistances = nil
	fire := func() int {
		before := m.HitPoints
		if !g.fireOverwatch(0, m) {
			t.Fatal("shot failed")
		}
		a := &g.arrows[len(g.arrows)-1]
		g.combat.applyProjectileDamage(a, "arrow", m, a.ID)
		return before - m.HitPoints
	}
	first := fire()
	if ch.DesignatedTargetID != m.ID || ch.DesignationFrames <= 0 {
		t.Fatal("successful impact did not designate target")
	}
	second := fire()
	trueDamage, _ := g.combat.weaponMasteryStrike(ch, def)
	if second != 2*(first-trueDamage)+trueDamage {
		t.Fatalf("marked critical damage=%d; ordinary=%d true=%d", second, first, trueDamage)
	}
	other := spawnMonsterAtTile(g, "wolf", 11, 10, tile)
	g.designateTarget(ch, other)
	if g.designationBonus(m) != 0 {
		t.Fatal("old target retained mark")
	}
	ch.DesignationFrames = 1
	g.updateTacticalClocks()
	if g.designationBonus(other) != 0 {
		t.Fatal("expired mark still active")
	}
}

func TestBallisticsAndMedicineUseSharedNumbers(t *testing.T) {
	g, _, ch, _ := sniperFixture(t, false)
	def, _ := config.GetWeaponDefinition("surveyors_rifle")
	for tier := 0; tier < 4; tier++ {
		ch.Skills[character.SkillBallistics].Mastery = character.SkillMastery(tier)
		ch.Skills[character.SkillFieldMedicine].Mastery = character.SkillMastery(tier)
		rangeTiles, speed := character.EffectiveWeaponFlight(def, ch)
		if rangeTiles != float64(def.Range+character.BallisticsRangeTiles(tier)) || math.Abs(speed-def.Physics.SpeedTiles*(1+float64(character.BallisticsSpeedPct(tier))/100)) > 1e-8 {
			t.Fatal("flight values diverged from mastery data")
		}
		ch.CurePoison()
		ch.ApplyPoison(1000)
		if ch.PoisonFramesRemaining != 1000*(100-character.FieldMedicinePoisonReductionPct(tier))/100 {
			t.Fatal("poison reduction diverged")
		}
		p := items.CreateItemFromYAML("mana_potion")
		ch.MaxSpellPoints = 1000
		ch.SpellPoints = 1
		g.party.Inventory = []items.Item{p}
		want := 1 + character.ConsumableRestore(ch, p.Attributes["mana_base"], p.Attributes["mana_personality_divisor"], true)
		if !g.UseConsumableFromInventory(0, 0) || ch.SpellPoints != want {
			t.Fatal("mana potion ignored Field Medicine")
		}
		text := GetItemTooltip(ch.Equipment[items.SlotMainHand], ch, g.combat, true)
		if !strings.Contains(text, fmt.Sprintf("Current Range: %.0f tiles", rangeTiles)) || !strings.Contains(text, fmt.Sprintf("Current Projectile Speed: %.1f tiles/s", speed)) {
			t.Fatal("tooltip ignored Ballistics")
		}
	}
}

func TestOverwatchPartialMovementAndOffCameraImpact(t *testing.T) {
	g, gl, ch, tile := sniperFixture(t, false)
	g.threading = threading.NewThreadingComponents(g.config)
	defer g.threading.Shutdown()
	g.camera.Angle = math.Pi
	m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
	m.BeginPlayerEngagement()
	m.State = monster.StatePursuing
	m.HitPoints = 10000
	m.PerfectDodge = 0
	m.ArmorClass = 0
	m.Resistances = nil
	for step := 0; step < 4; step++ {
		old := m.X
		m.X -= tile / 4
		g.observeOverwatchMovement(m, old, m.Y)
		if len(g.arrows) != (step+1)/4 {
			t.Fatal("partial movement counted as a whole tile")
		}
	}
	g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	before := m.HitPoints
	for i := 0; i < g.config.GetTPS()*2 && m.HitPoints == before; i++ {
		gl.updateProjectilesAndImpacts()
	}
	if m.HitPoints == before || ch.DesignatedTargetID != m.ID {
		t.Fatal("off-camera reaction did not hit/designate its target")
	}
	if g.camera.Angle != math.Pi {
		t.Fatal("reaction turned the player's camera")
	}
}

func TestOverwatchMainHandIgnoresDualWieldCursor(t *testing.T) {
	for _, tb := range []bool{false, true} {
		g, _, ch, tile := sniperFixture(t, tb)
		ch.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
		ch.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("iron_sword")
		ch.RTCooldown = 91
		ch.OffHandRTCooldown = 0
		ch.NextTBAttackOffHand = true
		m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
		if !g.fireOverwatch(0, m) || len(g.arrows) != 1 || g.arrows[0].BowKey != "surveyors_rifle" {
			t.Fatal("reaction chose the off-hand weapon")
		}
		if !ch.NextTBAttackOffHand || ch.RTCooldown != 91 || ch.OffHandRTCooldown != 0 {
			t.Fatal("reaction changed regular hand state")
		}
	}
}

func TestOverwatchIgnoresCosmeticShake(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("tb=%v", tb), func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, tb)
			for _, shake := range []float64{-3.25, 3.25} {
				g.screenShakeOffsetX = shake
				g.screenShakeOffsetY = -shake
				g.camera.X = g.tactics.x + shake
				g.camera.Y = g.tactics.y - shake
				if !g.overwatchReady(ch) {
					t.Fatal("cosmetic shake hid readiness without a party step")
				}
			}
		})
	}
}

func TestOverwatchFullMonsterPhase(t *testing.T) {
	for _, step := range []bool{false, true} {
		for _, acted := range []bool{false, true} {
			t.Run(fmt.Sprintf("step=%v/acted=%v", step, acted), func(t *testing.T) {
				g, gl, ch, tile := sniperFixture(t, true)
				m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
				m.BeginPlayerEngagement()
				m.State = monster.StatePursuing
				g.party.Members = append(g.party.Members, character.CreateCharacter("Ally", character.ClassKnight, g.config))
				g.selectedChar = 1
				ch.RTCooldown = 999
				if acted {
					g.partyActionsUsed = 1
				}
				if step {
					g.camera.Y += tile
					g.endPartyTurnAfterMovement()
				} else {
					g.endPartyTurn()
				}
				g.updateTacticalClocks()
				for n := 0; g.currentTurn == 1 && n < g.config.GetTPS()*3; n++ {
					g.frameCount++
					gl.updateMonstersTurnBased()
				}
				if g.currentTurn != 0 {
					t.Fatal("monster phase never completed")
				}
				if got := len(g.arrows) > 0; got != !step {
					t.Fatalf("reaction=%v after step=%v (monster x=%v)", got, step, m.X/tile)
				}
				if !g.overwatchReady(ch) {
					t.Fatal("next party round did not rearm sniper")
				}
			})
		}
	}
}

func TestScreenShakeRestoresLogicalPosition(t *testing.T) {
	for _, name := range []string{"idle", "shake", "external move"} {
		t.Run(name, func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, false)
			g.camera.X, g.camera.Y = 1023.99, 2047.99
			g.updateTacticalClocks()
			g.tactics.stationarySeconds = 10
			if name != "idle" {
				g.screenShake = 7.31
			}
			for frame := 0; frame < 100; frame++ {
				g.frameCount++
				g.camera.Angle = float64(frame) * 0.11
				x, y := g.camera.X, g.camera.Y
				restore := g.beginScreenShakeSwap()
				if name == "external move" {
					x, y = 30.1, 99.2
					g.camera.X, g.camera.Y = x, y
				}
				restore()
				if g.camera.X != x || g.camera.Y != y || g.screenShakeOffsetX != 0 || g.screenShakeOffsetY != 0 {
					t.Fatal("camera restoration drifted or overwrote an external move")
				}
				g.updateTacticalClocks()
				if name != "external move" && !g.overwatchReady(ch) {
					t.Fatal("drawing reset the stationary clock")
				}
			}
		})
	}
}

func TestOverwatchCombatLogIdentity(t *testing.T) {
	for _, reaction := range []bool{false, true} {
		for _, outcome := range []string{"hit", "kill", "dodge", "failed launch"} {
			t.Run(fmt.Sprintf("reaction=%v/%s", reaction, outcome), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
				m.HitPoints, m.MaxHitPoints = 10000, 10000
				m.PerfectDodge, m.ArmorClass, m.Resistances = 0, 0, nil
				switch outcome {
				case "kill":
					m.HitPoints = 1
				case "dodge":
					m.PerfectDodge = 1
					ch.Skills[character.SkillBlaster].Mastery = character.MasteryNovice
				case "failed launch":
					ch.StunFramesRemaining = 1
				}
				var fired bool
				if reaction {
					fired = g.fireOverwatch(0, m)
				} else {
					fired = g.combat.EquipmentMeleeAttack()
				}
				if fired != (outcome != "failed launch") {
					t.Fatal("unexpected launch result")
				}
				if !fired {
					if len(g.combatLogHistory) != 0 {
						t.Fatal("failed launch reported a shot")
					}
					return
				}
				launchEntries := len(g.combatLogHistory)
				if reaction && (launchEntries != 1 || !strings.Contains(g.combatLogHistory[0].Text, "fires Overwatch")) {
					t.Fatal("reaction launch is not identifiable")
				}
				a := &g.arrows[len(g.arrows)-1]
				if a.Overwatch != reaction || a.Label != "" {
					t.Fatal("reaction provenance changed bonus-bolt classification")
				}
				g.combat.applyProjectileDamage(a, "arrow", m, a.ID)
				messages := g.combatLogHistory[launchEntries:]
				if len(messages) == 0 || strings.Contains(messages[0].Text, "Overwatch") != reaction {
					t.Fatalf("impact lost reaction identity: %+v", messages)
				}
			})
		}
	}
}

func TestDesignationSharedEligibility(t *testing.T) {
	for _, name := range []string{"active", "expired", "dead target", "dead owner", "stunned owner", "reserve owner", "no skill", "replaced target", "multiple owners"} {
		t.Run(name, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			m := spawnMonsterAtTile(g, "wolf", 10, 10, tile)
			g.designateTarget(ch, m)
			want := character.DesignationCritPct(ch.SkillTier(character.SkillDesignateTarget))
			switch name {
			case "expired":
				ch.DesignationFrames = 1
				g.updateTacticalClocks()
				want = 0
			case "dead target":
				m.HitPoints = 0
				want = 0
			case "dead owner":
				ch.HitPoints = 0
				want = 0
			case "stunned owner":
				ch.StunFramesRemaining = 1
				want = 0
			case "reserve owner":
				g.party.Members = nil
				g.party.Reserve = []*character.MMCharacter{ch}
				want = 0
			case "no skill":
				delete(ch.Skills, character.SkillDesignateTarget)
				want = 0
			case "replaced target":
				g.designateTarget(ch, spawnMonsterAtTile(g, "wolf", 11, 10, tile))
				want = 0
			case "multiple owners":
				other := character.CreateCharacter("Other", character.ClassSniper, g.config)
				other.Skills[character.SkillDesignateTarget].Mastery = character.MasteryGrandMaster
				g.party.Members = append(g.party.Members, other)
				g.designateTarget(other, m)
				want = character.DesignationCritPct(other.SkillTier(character.SkillDesignateTarget))
			}
			if got := g.designationBonus(m); got != want {
				t.Fatalf("mark bonus=%d want %d", got, want)
			}
		})
	}
}

func TestCatalogSkillsPreserveMasteryAcrossSave(t *testing.T) {
	for tier := 0; tier < 4; tier++ {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			g, _, ch, _ := sniperFixture(t, false)
			for _, skill := range []character.SkillType{character.SkillOverwatch, character.SkillBallistics, character.SkillFieldMedicine, character.SkillDesignateTarget} {
				ch.Skills[skill].Mastery = character.SkillMastery(tier)
			}
			ch.AutoDrinkCooldown, ch.DesignationFrames, ch.DesignatedTargetID = 47, 93, "saved-mark"
			beforeCrit := g.combat.CalculateWeaponCritChance(ch.Equipment[items.SlotMainHand], ch)
			beforeRecovery := character.ConsumableRestore(ch, 100, 0, false)
			restored := restoreCharacterSave(buildCharacterSave(ch))
			for _, skill := range []character.SkillType{character.SkillOverwatch, character.SkillBallistics, character.SkillFieldMedicine, character.SkillDesignateTarget} {
				if !restored.HasSkill(skill) || restored.SkillTier(skill) != tier {
					t.Fatal("saved skill or mastery changed")
				}
			}
			if g.combat.CalculateWeaponCritChance(restored.Equipment[items.SlotMainHand], restored) != beforeCrit || character.ConsumableRestore(restored, 100, 0, false) != beforeRecovery || restored.AutoDrinkCooldown != 47 || restored.DesignationFrames != 93 || restored.DesignatedTargetID != "saved-mark" {
				t.Fatal("save round trip changed skill effects or ongoing state")
			}
		})
	}
}
