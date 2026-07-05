package main

// Characters page: a scrollable, full-detail panel per playable class - portrait,
// stats, skills, and starting spells/equipment shown inline with icons (no hover
// needed). Built by instantiating each class so it always matches the live game.

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	charPanelPad   = 14
	charPortraitSz = 128
	charIconSz     = 40
	charLineH      = 16
	charPanelGap   = 12
)

// panelRow is one rendered line in a character panel: either a text line
// (optionally a header) or an icon row (icon + label).
type panelRow struct {
	text     string
	header   bool
	hasIcon  bool
	iconKind contentKind
	iconKey  string
}

type charDetail struct {
	portrait    string // canonical hero name (lowercased) - preferred portrait art
	portraitKey string // class key - fallback art (recruits like paladin/druid)
	rows        []panelRow
}

// charTextCols is how many characters fit in the panel's right text column.
func charTextCols() int {
	w := windowWidth - 2*contentPad - charPortraitSz - 16 - 8
	c := w / 7
	if c < 16 {
		c = 16
	}
	return c
}

func buildCharacterDetails(cfg *config.Config) []charDetail {
	if cfg == nil {
		return nil
	}
	cols := charTextCols()
	var out []charDetail
	// One card per SHIPPED hero (starting party, captives, tavern recruits),
	// built through the SAME roster path the game uses (class kit + race
	// modifiers) - per-class approximations hid the recruits and their races.
	groups := []struct {
		label   string
		entries []config.RosterEntry
	}{
		{"Starting Party", cfg.Characters.StartingParty},
		{"Captive", cfg.Characters.Captives},
		{"Tavern Recruit", cfg.Characters.TavernRecruits},
	}
	for _, grp := range groups {
		for _, e := range grp.entries {
			ch := character.CreateRosterCharacter(e, cfg)
			if ch == nil {
				continue
			}
			class, _ := character.ClassFromKey(e.Class)
			key := e.Class
			// Portrait art is keyed by the lowercased hero name with a
			// class-key fallback - same resolution the game uses.
			d := charDetail{portrait: strings.ToLower(e.Name), portraitKey: key}
			txt := func(s string) { d.rows = append(d.rows, panelRow{text: s}) }
			hdr := func(s string) { d.rows = append(d.rows, panelRow{text: s, header: true}) }

			race := e.Race
			if race == "" {
				race = "human"
			}
			hdr(fmt.Sprintf("%s - %s", e.Name, titleCase(key)))
			txt(fmt.Sprintf("%s   -   Race: %s", grp.label, titleCase(strings.ReplaceAll(race, "_", " "))))
			txt("")
			for _, ln := range wrapTooltipLines(class.Blurb(), cols) {
				txt(ln)
			}
			txt("")
			txt(fmt.Sprintf("HP %d    SP %d    Level %d", ch.MaxHitPoints, ch.MaxSpellPoints, ch.Level))
			txt(fmt.Sprintf("Might %d   Intellect %d   Personality %d   Endurance %d",
				ch.Might, ch.Intellect, ch.Personality, ch.Endurance))
			txt(fmt.Sprintf("Accuracy %d   Speed %d   Luck %d", ch.Accuracy, ch.Speed, ch.Luck))

			// Skills (comma-joined, wrapped).
			var skills []string
			for _, st := range character.AllSkills {
				if sk, ok := ch.Skills[st]; ok && sk != nil {
					skills = append(skills, fmt.Sprintf("%s (%s)", st.String(), sk.Mastery.String()))
				}
			}
			txt("")
			hdr("Skills")
			if len(skills) == 0 {
				txt("(none)")
			}
			for _, ln := range wrapTooltipLines(strings.Join(skills, ",  "), cols) {
				txt(ln)
			}

			// Magic schools the character can use, then the starting spells (with
			// icons) grouped by school.
			schools := ch.GetAvailableSchools()
			spellRows := []panelRow{}
			for _, school := range schools {
				ms := ch.MagicSchools[school]
				if ms == nil {
					continue
				}
				for _, sid := range ms.KnownSpells {
					spellRows = append(spellRows, panelRow{
						hasIcon: true, iconKind: cardSpell, iconKey: string(sid),
						text: fmt.Sprintf("%s - %s", school.DisplayName(), spellDisplayName(string(sid))),
					})
				}
			}
			if len(schools) > 0 {
				schoolNames := make([]string, 0, len(schools))
				for _, s := range schools {
					schoolNames = append(schoolNames, s.DisplayName())
				}
				txt("")
				hdr("Magic schools")
				for _, ln := range wrapTooltipLines(strings.Join(schoolNames, ",  "), cols) {
					txt(ln)
				}
			}
			if len(spellRows) > 0 {
				txt("")
				hdr("Starting spells")
				d.rows = append(d.rows, spellRows...)
			}

			// Starting equipment (with icons). The equipped spell slot is skipped -
			// it just duplicates a spell already listed under "Starting spells".
			equipRows := []panelRow{}
			for _, s := range equipSlotOrder {
				if s.slot == items.SlotSpell {
					continue
				}
				it, ok := ch.Equipment[s.slot]
				if !ok || it.Name == "" {
					continue
				}
				kind, key := cardItem, itemKeyByName(it.Name)
				if s.slot == items.SlotMainHand {
					kind, key = cardWeapon, weaponKeyByName(it.Name)
				}
				equipRows = append(equipRows, panelRow{
					hasIcon: true, iconKind: kind, iconKey: key,
					text: fmt.Sprintf("%s - %s", s.label, it.Name),
				})
			}
			txt("")
			hdr("Starting equipment")
			if len(equipRows) == 0 {
				txt("(none)")
			}
			d.rows = append(d.rows, equipRows...)

			out = append(out, d)
		}
	}
	return out
}

func itemKeyByName(name string) string {
	if _, key, ok := config.GetItemDefinitionByName(name); ok {
		return key
	}
	return ""
}

func weaponKeyByName(name string) string {
	if _, key, ok := config.GetWeaponDefinitionByName(name); ok {
		return key
	}
	return ""
}

func rowHeight(r panelRow) int {
	if r.hasIcon {
		return charIconSz + 6
	}
	return charLineH
}

func (v *viewer) charPanelHeight(d *charDetail) int {
	h := 0
	for _, r := range d.rows {
		h += rowHeight(r)
	}
	if h < charPortraitSz {
		h = charPortraitSz
	}
	return h + 2*charPanelPad
}

func (v *viewer) drawCharactersPage(screen *ebiten.Image) {
	areaX := contentPad
	areaY := pageBarHeight + contentPad
	areaW := windowWidth - 2*contentPad
	areaH := windowHeight - areaY - contentPad

	clip := screen.SubImage(image.Rect(areaX, areaY, areaX+areaW, areaY+areaH)).(*ebiten.Image)
	clip.Fill(color.RGBA{20, 20, 30, 255})

	y := areaY - v.pageScroll[pageChars]
	for i := range v.charDetails {
		d := &v.charDetails[i]
		h := v.charPanelHeight(d)
		if y+h >= areaY && y < areaY+areaH {
			v.drawCharPanel(clip, d, areaX, y, areaW, h)
		}
		y += h + charPanelGap
	}
}

func (v *viewer) drawCharPanel(dst *ebiten.Image, d *charDetail, x, y, w, h int) {
	drawFilledRect(dst, x, y, w, h, color.RGBA{30, 30, 42, 255})
	drawRectBorder(dst, x, y, w, h, 1, color.RGBA{72, 72, 96, 255})

	// Portrait on the left.
	px, py := x+charPanelPad, y+charPanelPad
	if img := v.charPortrait(d.portrait, d.portraitKey); img != nil {
		drawImageScaled(dst, img, px, py, charPortraitSz, charPortraitSz)
	} else {
		drawFilledRect(dst, px, py, charPortraitSz, charPortraitSz, color.RGBA{50, 50, 66, 255})
		drawRectBorder(dst, px, py, charPortraitSz, charPortraitSz, 1, color.RGBA{80, 80, 100, 255})
		ebitenutil.DebugPrintAt(dst, "?", px+charPortraitSz/2-3, py+charPortraitSz/2-7)
	}

	rx := x + charPanelPad + charPortraitSz + 16
	rw := w - (charPanelPad + charPortraitSz + 16) - charPanelPad
	cols := charTextCols()
	ry := y + charPanelPad
	for _, r := range d.rows {
		if r.hasIcon {
			if icon := v.iconKindKey(r.iconKind, r.iconKey); icon != nil {
				drawImageScaled(dst, icon, rx, ry, charIconSz, charIconSz)
			} else {
				drawFilledRect(dst, rx, ry, charIconSz, charIconSz, color.RGBA{52, 52, 68, 255})
				drawRectBorder(dst, rx, ry, charIconSz, charIconSz, 1, color.RGBA{80, 80, 100, 255})
			}
			ebitenutil.DebugPrintAt(dst, truncate(r.text, cols), rx+charIconSz+8, ry+charIconSz/2-7)
			ry += charIconSz + 6
		} else {
			if r.header {
				drawFilledRect(dst, rx-4, ry-2, rw, charLineH+2, color.RGBA{48, 48, 72, 255})
			}
			ebitenutil.DebugPrintAt(dst, truncate(r.text, cols), rx, ry)
			ry += charLineH
		}
	}
}

// maxCharactersScroll mirrors maxContentScroll for the custom Characters page.
func (v *viewer) maxCharactersScroll() int {
	areaY := pageBarHeight + contentPad
	areaH := windowHeight - areaY - contentPad
	total := 0
	for i := range v.charDetails {
		total += v.charPanelHeight(&v.charDetails[i]) + charPanelGap
	}
	if total <= areaH {
		return 0
	}
	return total - areaH
}

// iconKindKey loads a content icon by kind+key (reuses the card icon cache).
func (v *viewer) iconKindKey(kind contentKind, key string) *ebiten.Image {
	c := contentCard{kind: kind, key: key}
	return v.iconForCard(&c)
}

// charPortrait loads a class portrait, preferring the large "_full" art. It
// tries the canonical hero name first, then the class-key fallback (recruits
// like paladin/druid ship art under the class key) - the same resolution the
// game uses (basePortraitSpriteName). Cached.
func (v *viewer) charPortrait(name, fallbackKey string) *ebiten.Image {
	cacheKey := "portrait:" + name + "|" + fallbackKey
	if img, ok := v.iconCache[cacheKey]; ok {
		return img
	}
	for _, base := range []string{name, fallbackKey} {
		if base == "" {
			continue
		}
		for _, suffix := range []string{"_full", ""} {
			if path, ok := graphics.ResolveSpritePath(base + suffix); ok {
				if img, _, err := ebitenutil.NewImageFromFile(path); err == nil {
					v.iconCache[cacheKey] = img
					return img
				}
			}
		}
	}
	v.iconCache[cacheKey] = nil
	return nil
}
