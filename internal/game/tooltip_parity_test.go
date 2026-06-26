package game

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Why this test exists — read internal/character/cardtemplate.go's "two parallel
// builders" note first. The in-game card (buildWeaponTooltipUnified /
// buildSpellTooltipUnified, package game, CombatSystem numbers) and the map-editor
// card (character.WeaponCardSections / SpellCardSections, character-independent
// formulas) are built by SEPARATE functions because the editor cannot import the
// combat package. They share section order, EffectLines and rule helpers, but the
// term-EXISTENCE logic is written twice — which is exactly where they drifted
// (Ray of Light's missing Personality term, the absent Arms Master line, the
// stub crit block).
//
// Raw-string comparison is wrong here: the cards intentionally differ in STYLE —
// the game prints values ("Intellect (40 / 3): +13") and a numeric Total, the
// editor prints formulas ("Intellect / 3: scales") and no Total. So we extract a
// normalized mechanicSkeleton from each rendered card and compare only what must
// agree: the ordered set of scaling stats, the shared EffectLines, the damage
// type, and presence of the mastery / arms-master / crit / cooldown blocks.
// Adding a mechanic to one builder and not the other makes a skeleton field
// diverge and fails the suite — the architectural agreement becomes a contract.

type mechanicSkeleton struct {
	scalingStats  []string        // ORDERED "Stat/divisor" terms (order is meaningful)
	effects       map[string]bool // shared FilteredSpellEffectLines / weapon EffectLines (style-independent)
	hasMastery    bool            // school/weapon mastery scaling present
	hasArmsMaster bool            // Arms Master contribution present
	hasCooldown   bool            // a cooldown line is shown
	rules         map[string]bool // normalized RULES-section tokens (wording-independent)
	attack        map[string]bool // weapon ATTACK tokens (arc/speed/hitbox/max/multiplier) minus absolute cooldown
	casting       map[string]bool // spell CASTING delivery tokens (range/proj-speed/hitbox/target); cost+cooldown excluded
	crit          map[string]bool // normalized crit components (base / chance / luck / gm-weapon / gm-arms / mult)
}

var (
	reGameStat   = regexp.MustCompile(`([A-Za-z]+) \(\d+ / (\d+)\): \+`) // "Intellect (40 / 3): +13"
	reEditorStat = regexp.MustCompile(`([A-Za-z]+) / (\d+): scales`)     // "Intellect / 3: scales"
)

// extractSkeleton normalizes a rendered card (either style) into the comparable
// mechanic skeleton. It deliberately drops the value-vs-formula-divergent lines
// (Totals, Normal subtotal, duration decomposition, the cooldown SECONDS, crit
// numbers) and keeps only mechanic identity.
func extractSkeleton(card string) mechanicSkeleton {
	sk := mechanicSkeleton{
		effects: map[string]bool{}, rules: map[string]bool{},
		attack: map[string]bool{}, casting: map[string]bool{}, crit: map[string]bool{},
	}
	lines := strings.Split(card, "\n")
	section := ""
	for _, raw := range lines {
		ln := strings.TrimSpace(raw)
		if ln == "" {
			// RenderCardLines separates sections with a blank line and never puts
			// one inside a section, so a blank ends the current section. Resetting
			// here stops trailing footer lines the game appends after the card
			// (Value/flavor/[Shift] hint) from being mis-attributed to RULES.
			section = ""
			continue
		}
		if isSectionHeader(ln) {
			section = ln
			continue
		}
		// Scaling terms, in the order they appear, from any section.
		if m := reGameStat.FindStringSubmatch(ln); m != nil {
			sk.scalingStats = append(sk.scalingStats, m[1]+"/"+m[2])
		} else if m := reEditorStat.FindStringSubmatch(ln); m != nil {
			sk.scalingStats = append(sk.scalingStats, m[1]+"/"+m[2])
		}
		// Mastery means the DAMAGE/HEAL mastery CONTRIBUTION specifically — scoped to
		// the damage sections so the EFFECTS duration-mastery line ("Mastery: +20%
		// duration per tier") can't mask a removed damage-mastery term, and so the
		// word appearing in RULES/CRITICAL text never counts.
		if strings.Contains(ln, "Mastery") &&
			(section == "DAMAGE" || section == "HEALING" || section == "DAMAGE PER TICK") {
			sk.hasMastery = true
		}
		// Arms Master must be the DAMAGE contribution — scoped, else the CRITICAL
		// "GM Arms Master" line would keep it true after the damage term is dropped.
		if section == "DAMAGE" && strings.Contains(ln, "Arms Master") {
			sk.hasArmsMaster = true
		}
		if strings.Contains(strings.ToLower(ln), "cooldown") {
			sk.hasCooldown = true
		}
		// Weapon ATTACK mechanics (arc, attack-speed multiplier, projectile speed,
		// hitbox, max projectiles, range) — the shared/identical lines. The ABSOLUTE
		// "RT Cooldown: Xs" is excluded (editor has no Speed to compute it).
		if section == "ATTACK" && !strings.HasPrefix(ln, "RT Cooldown:") {
			sk.attack[ln] = true
		}
		// Spell CASTING delivery mechanics — whitelist the lines whose TEXT is shared
		// (Range/Projectile Speed/Hitbox/Target). Cost (Meditation discount), the
		// cooldown seconds and the Speed/staff notes legitimately diverge → excluded.
		if section == "CASTING" {
			for _, p := range []string{"Range:", "Projectile Speed:", "Hitbox:", "Target:"} {
				if strings.HasPrefix(ln, p) {
					sk.casting[ln] = true
				}
			}
		}
		// Crit COMPONENTS (not just the header): both styles name the same parts.
		if section == "CRITICAL" {
			if strings.Contains(ln, "Base") { // game breakdown "Base: 12%" / editor "Base Chance: 12%"
				sk.crit["base"] = true
			}
			if strings.Contains(ln, "Chance") { // game total "Chance: 34%" / editor "Base Chance:"
				sk.crit["chance"] = true
			}
			if strings.Contains(ln, "Luck") {
				sk.crit["luck"] = true
			}
			if strings.Contains(ln, "weapon: +") { // "GM weapon: +7%" / "Grandmaster weapon: +7%"
				sk.crit["gm-weapon"] = true
			}
			if strings.Contains(ln, "Arms Master: +") { // "GM Arms Master: +5%" / "Grandmaster Arms Master: +5%"
				sk.crit["gm-arms"] = true
			}
		}
		// EFFECTS lines are fed from the shared EffectLines in BOTH builders, minus
		// the legitimately style-divergent duration decomposition.
		if section == "EFFECTS" && !isStyleDivergent(ln) {
			sk.effects[ln] = true
		}
		if section == "RULES" {
			sk.rules[normalizeRule(ln)] = true
		}
	}
	// The ×CritDamageMultiplier component: game prints "Critical Damage: N" (DAMAGE),
	// the editor prints "Critical hits deal ×N damage" (CRITICAL) — same mechanic.
	if strings.Contains(card, "Critical Damage:") || strings.Contains(card, "Critical hits deal ×") {
		sk.crit["mult"] = true
	}
	return sk
}

// normalizeRule collapses the few RULES lines whose WORDING legitimately differs
// between the two builders (game "GM:" vs editor "Grandmaster:"; game "this
// strike ignores" vs editor "strikes ignore"; the per-skill GM-dodge variant) to
// a canonical form, so the rule-SET comparison catches a genuinely MISSING rule
// without false-firing on cosmetic wording.
func normalizeRule(ln string) string {
	s := strings.ToLower(strings.TrimSpace(ln))
	s = strings.ReplaceAll(s, "gm:", "grandmaster:")
	s = strings.ReplaceAll(s, "this strike ignores", "strikes ignore")
	if strings.Contains(s, "ignore perfect dodge") { // weapon GM-dodge: collapse all variants
		return "grandmaster: strikes ignore perfect dodge"
	}
	return s
}

// isSectionHeader matches the UPPERCASE titles (with a couple of two-word ones).
func isSectionHeader(ln string) bool {
	switch ln {
	case "ATTACK", "DAMAGE", "HEALING", "ZONE", "EFFECTS", "RULES", "CRITICAL",
		"CASTING", "PLACEMENT", "EFFECT", "DAMAGE PER TICK", "DEFENSE", "USAGE":
		return true
	}
	return false
}

// isStyleDivergent flags the EFFECTS lines that the two builders render
// differently BY DESIGN (value vs formula) — duration decomposition.
func isStyleDivergent(line string) bool {
	return strings.HasPrefix(line, "Base Duration:") ||
		strings.HasPrefix(line, "Current Duration:") ||
		strings.HasPrefix(line, "Current damage bonus:") ||
		strings.HasPrefix(line, "Current physical damage bonus:") ||
		strings.HasPrefix(line, "Current reduction:") ||
		strings.HasPrefix(line, "Current resistance:") ||
		strings.HasPrefix(line, "Current stat bonus:") ||
		strings.HasPrefix(line, "Mastery:") || // editor "Mastery: +20% duration per tier"
		strings.Contains(line, " Mastery — ") // game "<School> Mastery — Tier: +N%"
}

// diff reports comparable-field mismatches. includeCooldown is false for weapons:
// the editor weapon card can't show an ABSOLUTE cooldown (no character → no Speed
// curve), only the relative attack-speed multiplier, so its presence legitimately
// differs from the game's "RT Cooldown: Xs". Spells DO carry an authored
// cooldown_seconds the editor can render, so there it's compared.
func (s mechanicSkeleton) diff(other mechanicSkeleton, includeCooldown bool) []string {
	var out []string
	if !equalSeq(s.scalingStats, other.scalingStats) {
		out = append(out, "scalingStats "+sliceStr(s.scalingStats)+" != "+sliceStr(other.scalingStats))
	}
	if !equalSet(s.effects, other.effects) {
		out = append(out, "effects "+setStr(s.effects)+" != "+setStr(other.effects))
	}
	if !equalSet(s.rules, other.rules) {
		out = append(out, "rules "+setStr(s.rules)+" != "+setStr(other.rules))
	}
	if !equalSet(s.attack, other.attack) {
		out = append(out, "attack "+setStr(s.attack)+" != "+setStr(other.attack))
	}
	if !equalSet(s.casting, other.casting) {
		out = append(out, "casting "+setStr(s.casting)+" != "+setStr(other.casting))
	}
	if !equalSet(s.crit, other.crit) {
		out = append(out, "crit "+setStr(s.crit)+" != "+setStr(other.crit))
	}
	if s.hasMastery != other.hasMastery {
		out = append(out, "hasMastery differs")
	}
	if s.hasArmsMaster != other.hasArmsMaster {
		out = append(out, "hasArmsMaster differs")
	}
	if includeCooldown && s.hasCooldown != other.hasCooldown {
		out = append(out, "hasCooldown differs")
	}
	return out
}

func TestCardParity_WeaponsGameVsEditor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := gmReferenceChar(cs.game.config)

	processed := 0
	for key := range config.GlobalWeapons.Weapons {
		def, ok := config.GetWeaponDefinition(key)
		if !ok || def == nil {
			t.Fatalf("weapon %q is in GlobalWeapons but GetWeaponDefinition failed", key)
		}
		processed++
		gameCard := GetItemTooltip(items.CreateWeaponFromYAML(key), char, cs, true)
		editorCard := strings.Join(character.RenderCardLines(
			character.WeaponCardSections(def, character.ArmorPhysicalReductionDivisor), true), "\n")

		if d := extractSkeleton(gameCard).diff(extractSkeleton(editorCard), false); len(d) > 0 {
			t.Errorf("weapon %q game/editor drift:\n  %s\n--- game ---\n%s\n--- editor ---\n%s",
				key, strings.Join(d, "\n  "), gameCard, editorCard)
		}
	}
	// Fail-fast on silent under-coverage: every loaded weapon must be checked.
	if processed != len(config.GlobalWeapons.Weapons) {
		t.Fatalf("checked %d weapons, expected %d", processed, len(config.GlobalWeapons.Weapons))
	}
}

func TestCardParity_SpellsGameVsEditor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := gmReferenceChar(cs.game.config)

	processed := 0
	for key := range config.GlobalSpells.Spells {
		def, ok := config.GetSpellDefinition(key)
		if !ok || def == nil {
			t.Fatalf("spell %q is in GlobalSpells but GetSpellDefinition failed", key)
		}
		sd, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatalf("spell %q is in GlobalSpells but GetSpellDefinitionByID failed: %v", key, err)
		}
		processed++
		// Monster-only spells use a distinct editor profile (no player formula) and
		// are never shown in the in-game spellbook — assert the Monster profile
		// instead of player parity.
		if def.MonsterOnly {
			assertMonsterProfile(t, key, def, sd)
			continue
		}
		gameCard := buildSpellTooltipUnified(sd, char, cs, true)
		editorCard := strings.Join(character.RenderCardLines(
			character.SpellCardSections(key, def, sd, character.ArmorPhysicalReductionDivisor), true), "\n")

		if d := extractSkeleton(gameCard).diff(extractSkeleton(editorCard), true); len(d) > 0 {
			t.Errorf("spell %q game/editor drift:\n  %s\n--- game ---\n%s\n--- editor ---\n%s",
				key, strings.Join(d, "\n  "), gameCard, editorCard)
		}
	}
	// Fail-fast on silent under-coverage: every loaded spell must be checked.
	if processed != len(config.GlobalSpells.Spells) {
		t.Fatalf("checked %d spells, expected %d", processed, len(config.GlobalSpells.Spells))
	}
}

// TestCardParity_TrapsGameVsEditor covers the third combat-ability card pair
// (buildTrapTooltipUnified vs character.TrapCardSections). Traps scale on a
// COMPOUND stat — "(Intellect + Accuracy) / N" — that the weapon/spell scaling
// regexes don't model, so instead of the generic skeleton this asserts each card
// surfaces what the YAML def declares (a drift in EITHER card vs the source of
// truth fails). The SP Cost (GM-Meditation-discounted in-game, raw in the editor)
// and the LOCKED level-gate line are intentional value/state divergences and are
// not compared — same rationale as the spell-cost / weapon-cooldown exclusions.
func TestCardParity_TrapsGameVsEditor(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := gmReferenceChar(cs.game.config)

	keys := config.TrapKeysOrdered()
	if len(keys) == 0 {
		t.Fatal("no traps loaded — newTestCombatSystemWithConfig must LoadTrapConfig")
	}
	processed := 0
	for _, key := range keys {
		def, ok := config.GetTrapDefinition(key)
		if !ok || def == nil {
			t.Fatalf("trap %q is in TrapKeysOrdered but GetTrapDefinition failed", key)
		}
		processed++
		gameCard := buildTrapTooltipUnified(key, def, char, cs, true)
		editorCard := strings.Join(character.RenderCardLines(
			character.TrapCardSections(def, config.TrapPlaceRangeTiles, config.MaxTrapsPerOwner,
				character.ArmorPhysicalReductionDivisor), true), "\n")

		// A mechanic the def declares must appear in BOTH cards — requiring presence
		// (not mere equality) catches the case where BOTH builders drop it.
		mustMatch := func(when bool, name, sub string) {
			if !when {
				return
			}
			g, e := strings.Contains(gameCard, sub), strings.Contains(editorCard, sub)
			if !g || !e {
				t.Errorf("trap %q: %s missing from %s (game=%v editor=%v)\n--- game ---\n%s\n--- editor ---\n%s",
					key, name, missingSide(g, e), g, e, gameCard, editorCard)
			}
		}
		mustMatch(def.DamageBase > 0, "damage scaling", "Intellect + Accuracy")
		mustMatch(def.DamageBase > 0, "trapper bonus", "Trapper")
		mustMatch(def.StunTurns > 0, "stun", "Stun")
		mustMatch(def.RootTurns > 0, "root", "Root")
		mustMatch(def.AoeRadiusTiles > 0, "AoE", "AoE")
		// Element/AoE EFFECTS line is the shared DamageTypeAoELine — must be identical.
		want := character.DamageTypeAoELine(def.Element, def.AoeRadiusTiles)
		if def.DamageBase > 0 && (!strings.Contains(gameCard, want) || !strings.Contains(editorCard, want)) {
			t.Errorf("trap %q: element/AoE line %q missing from a card", key, want)
		}
	}
	if processed != len(keys) {
		t.Fatalf("checked %d traps, expected %d", processed, len(keys))
	}
}

// missingSide names which card(s) lacked an expected substring.
func missingSide(inGame, inEditor bool) string {
	switch {
	case !inGame && !inEditor:
		return "BOTH cards"
	case !inGame:
		return "the game card"
	default:
		return "the editor card"
	}
}

// assertMonsterProfile checks a monster-only spell's editor card describes
// monster mechanics, never the player formula.
func assertMonsterProfile(t *testing.T, key string, def *config.SpellDefinitionConfig, sd spells.SpellDefinition) {
	t.Helper()
	card := strings.Join(character.RenderCardLines(character.MonsterSpellCardSections(def, sd), true), "\n")
	if !strings.Contains(card, "Cast by monsters only") {
		t.Errorf("monster spell %q must declare monster-only:\n%s", key, card)
	}
	for _, leak := range []string{"Cost:", "Intellect /", "Mastery:", "Base ("} {
		if strings.Contains(card, leak) {
			t.Errorf("monster spell %q leaks player formula %q:\n%s", key, leak, card)
		}
	}
}

// gmReferenceChar is a "grandmaster everything" caster: every skill and magic
// school at GM, high stats. The editor formula card always lists the skill-gated
// lines (mastery, true damage, arms master); the game card only shows them when
// the caster actually has the skill — so without GM skills the comparison would
// false-positive on legitimately absent lines.
func gmReferenceChar(cfg *config.Config) *character.MMCharacter {
	c := character.CreateCharacter("Parity", character.ClassSorcerer, cfg)
	for _, s := range character.AllSkills {
		c.Skills[s] = &character.Skill{Mastery: character.MasteryGrandMaster}
	}
	for _, id := range character.AllMagicSchools {
		c.MagicSchools[id] = &character.MagicSkill{Mastery: character.MasteryGrandMaster}
	}
	c.Might, c.Intellect, c.Personality = 40, 40, 40
	c.Endurance, c.Accuracy, c.Speed, c.Luck = 40, 40, 40, 40
	return c
}

func equalSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func sliceStr(s []string) string { return "[" + strings.Join(s, " ") + "]" }

func setStr(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return "{" + strings.Join(ks, " | ") + "}"
}
