package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// A color-key list entry that resolves to no sprite, or names an illustration
// the key never touches, aborts the boot instead of silently recolouring art.
// The shipped config is the positive control.
func TestApplySpriteColorKeyRejectsUnresolvedNames(t *testing.T) {
	base := loadTestConfig(t)
	t.Chdir("../..")
	for _, tc := range []struct {
		name  string
		edit  func(*config.ColorKeyConfig)
		panic string
	}{
		{"shipped", func(*config.ColorKeyConfig) {}, ""},
		{"edge-only typo", func(ck *config.ColorKeyConfig) { ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, "lich_kng") }, `edge_only_despill: sprite "lich_kng" not found`},
		{"exclusion typo", func(ck *config.ColorKeyConfig) { ck.DespillExclusions = append(ck.DespillExclusions, "lich_kng") }, `despill_exclusions: sprite "lich_kng" not found`},
		{"edge-only illustration", func(ck *config.ColorKeyConfig) { ck.EdgeOnlyDespill = append(ck.EdgeOnlyDespill, "lysander_full") }, `edge_only_despill: sprite "lysander_full" is an illustration`},
		{"bad glob", func(ck *config.ColorKeyConfig) { ck.Illustrations = append(ck.Illustrations, "camp_[") }, `illustrations: pattern "camp_["`},
		{"glob without its folder", func(ck *config.ColorKeyConfig) { ck.Illustrations = append(ck.Illustrations, "camp_*") }, `illustrations: pattern "camp_*" matches no sprite`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *base
			ck := cfg.Graphics.ColorKey
			ck.EdgeOnlyDespill = append([]string(nil), ck.EdgeOnlyDespill...)
			ck.DespillExclusions = append([]string(nil), ck.DespillExclusions...)
			ck.Illustrations = append([]string(nil), ck.Illustrations...)
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
