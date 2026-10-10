package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/shaders"
	"ugataima/internal/world"
)

const (
	turnModeIconSize = 64
	domainIconSize   = 48
	modeDomainGap    = 16
)

func (g *MMGame) elementalDomainAt(x, y float64) string {
	if g.config == nil || world.GlobalWorldManager == nil {
		return ""
	}
	ts := g.config.GetTileSize()
	biome := g.biomeAtTile(TileIndex(x, ts), TileIndex(y, ts))
	return world.GlobalWorldManager.Biomes[biome].ElementalAttackSchool
}

func (ui *UISystem) modeDomainIconLayout() (domain, mode layoutRect) {
	mode = layoutRect{ui.game.config.GetScreenWidth() - 10 - turnModeIconSize, 10, turnModeIconSize, turnModeIconSize}
	domain = layoutRect{mode.x - modeDomainGap - domainIconSize, mode.y + (mode.h-domainIconSize)/2, domainIconSize, domainIconSize}
	return
}

func (ui *UISystem) drawModeDomainIcons(dst *ebiten.Image) {
	if ui.game.sprites == nil {
		return
	}
	mode, label := "rt", uitext.Text("hud.real_time")
	if ui.game.turnBasedMode {
		mode, label = "tb", uitext.Text("hud.turn_based")
	}
	domainRect, modeRect := ui.modeDomainIconLayout()
	drawImageScaled(dst, ui.modeDomainIcon("hud_mode_"+mode), modeRect.x, modeRect.y, modeRect.w, modeRect.h)
	mx, my := uiCursorPosition()
	tooltip := func(r layoutRect, text string) {
		if !ui.modalLayerOwnsInput() && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()) {
			ui.queueTooltip([]string{text}, mx+12, my+8)
		}
	}
	tooltip(modeRect, label)
	if ui.game.camera == nil {
		return
	}
	domain := ui.game.elementalDomainAt(ui.game.camera.X, ui.game.camera.Y)
	if domain == "" {
		return
	}
	drawImageScaled(dst, ui.modeDomainIcon("hud_domain_"+domain), domainRect.x, domainRect.y, domainRect.w, domainRect.h)
	tooltip(domainRect, uitext.Text("hud.domain", character.MagicSchoolID(domain).DisplayName()))
}

type modeDomainIconCache struct {
	source, image *ebiten.Image
}

// The authored HUD symbols have a black backing. Key it once at native size,
// then use the normal shared scaler so their transparent edges filter cleanly.
func (ui *UISystem) modeDomainIcon(name string) *ebiten.Image {
	src := ui.game.sprites.GetSprite(name)
	if src == nil {
		return nil
	}
	if cached := ui.modeDomainIcons[name]; cached.source == src {
		return cached.image
	}
	if ui.modeDomainIconShader == nil {
		var err error
		ui.modeDomainIconShader, err = ebiten.NewShader([]byte(shaders.Source("hud_icon_key.kage")))
		if err != nil {
			panic(err)
		}
		ui.modeDomainIcons = make(map[string]modeDomainIconCache)
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	keyed := ebiten.NewImage(w, h)
	opts := &ebiten.DrawRectShaderOptions{}
	opts.Images[0] = src
	keyed.DrawRectShader(w, h, ui.modeDomainIconShader, opts)
	if old := ui.modeDomainIcons[name]; old.image != nil {
		old.image.Deallocate()
	}
	ui.modeDomainIcons[name] = modeDomainIconCache{src, keyed}
	return keyed
}
