package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// A color-key list entry that resolves to no sprite, a HUD alias (keyed as its
// framed icon, so the entry would do nothing) or a family on both despill lists
// aborts the boot instead of silently recolouring art or doing nothing, whether
// the key is enabled or not. The shipped config is the positive control.
func TestApplySpriteColorKeyRejectsUnresolvedNames(t *testing.T) {
	base := loadTestConfig(t)
	t.Chdir("../..")
	old := config.GlobalIconFrames
	t.Cleanup(func() { config.GlobalIconFrames = old })
	if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
		t.Fatal(err)
	}
	icons := slices.Sorted(maps.Keys(config.GlobalIconFrames.Icons))
	if len(icons) == 0 {
		t.Fatal("no framed content icon to alias")
	}
	alias := graphics.NewSpriteManager().HUDIconName(icons[0])
	for _, tc := range []struct {
		name  string
		edit  func(*config.ColorKeyConfig)
		panic string
	}{
		{"shipped", func(*config.ColorKeyConfig) {}, ""},
		{"edge-only typo", func(ck *config.ColorKeyConfig) { ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, "lich_kng") }, `edge_only_despill: sprite "lich_kng" not found`},
		{"full typo", func(ck *config.ColorKeyConfig) { ck.FullDespill = append(ck.FullDespill, "lich_kng") }, `full_despill: sprite "lich_kng" not found`},
		{"HUD alias", func(ck *config.ColorKeyConfig) { ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, alias) },
			fmt.Sprintf("edge_only_despill: sprite %q is the HUD alias of %q", alias, icons[0])},
		{"both lists", func(ck *config.ColorKeyConfig) {
			ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, "alien_enforcer")
			ck.FullDespill = append(ck.FullDespill, "alien_enforcer")
		}, `full_despill: sprite "alien_enforcer" is also in edge_only_despill`},
		{"both lists by sheet", func(ck *config.ColorKeyConfig) {
			ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, "alien_enforcer")
			ck.FullDespill = append(ck.FullDespill, "alien_enforcer_dying_l")
		}, `full_despill: sprite "alien_enforcer_dying_l" is also in edge_only_despill`},
	} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/enabled=%v", tc.name, enabled), func(t *testing.T) {
				cfg := *base
				ck := cfg.Graphics.ColorKey
				ck.Enabled = enabled
				ck.EdgeOnlyDespill = append([]string(nil), ck.EdgeOnlyDespill...)
				ck.FullDespill = append([]string(nil), ck.FullDespill...)
				tc.edit(&ck)
				cfg.Graphics.ColorKey = ck
				got := func() (msg string) {
					defer func() {
						if r := recover(); r != nil {
							msg = fmt.Sprint(r)
						}
					}()
					ApplySpriteColorKey(graphics.NewSpriteManager(), &cfg)
					return ""
				}()
				if tc.panic == "" && got != "" || tc.panic != "" && !strings.Contains(got, tc.panic) {
					t.Fatalf("panic %q, want %q", got, tc.panic)
				}
			})
		}
	}
}
