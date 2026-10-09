package main

import (
	"fmt"

	"ugataima/internal/config"
	"ugataima/internal/game"
)

// FX page: live preview of the game's special effects. The heavy lifting is
// game.FxPreview - a sandbox MMGame whose real combat/render code plays the
// selected effect - so the editor stays a thin list + viewport around it.

var fxPage struct {
	preview *game.FxPreview
	items   []game.FxItem
	selIdx  int
	scroll  int
	initErr string
}

// ensureFXPage lazily builds the sandbox on first tab open, so editor startup
// cost is unchanged and an FX init failure degrades to an on-page message.
func (v *viewer) ensureFXPage() {
	if fxPage.preview != nil || fxPage.initErr != "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			fxPage.initErr = fmt.Sprintf("FX sandbox failed to start: %v", r)
		}
	}()
	p, err := game.NewFxPreview(config.GlobalConfig)
	if err != nil {
		fxPage.initErr = err.Error()
		return
	}
	fxPage.preview = p
	fxPage.items = p.Items()
	if len(fxPage.items) > 0 {
		p.Select(fxPage.items[0])
		rows := v.fxCatalogRows()
		if i := catalogSelectedRow(rows, v.browser.fxSelection, fxPage.selIdx); i >= 0 {
			l, _ := v.fxCatalogLayout()
			fxPage.scroll = clampInt(i*catalogRowHeight-l.h/2, 0, max(0, len(rows)*catalogRowHeight-l.h))
		}
	}
}

func fxKindTag(k game.FxKind) string {
	switch k {
	case game.FxSpell:
		return "[spell]"
	case game.FxWeapon:
		return "[weapon]"
	case game.FxTrap:
		return "[trap]"
	case game.FxTile:
		return "[tile]"
	case game.FxCard:
		return "[card]"
	case game.FxStatus:
		return "[status]"
	}
	return "[?]"
}
