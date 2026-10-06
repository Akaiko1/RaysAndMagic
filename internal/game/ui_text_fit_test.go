package game

import (
	"fmt"
	"strings"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/quests"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// Every merchant price form must FIT the box it is drawn in, and the box must
// stop short of the next column. Measured, not eyeballed: a compound price
// ("scale + gold") used to overrun its cell and collide with the neighbour.
func TestMerchantPriceLabelsFitTheirBox(t *testing.T) {
	pitch := merchantIconSize + merchantIconGapX
	if merchantPriceBoxW >= pitch {
		t.Fatalf("price box %d >= column pitch %d - neighbouring prices would touch", merchantPriceBoxW, pitch)
	}
	// The box is centred on the icon, so it may not reach either neighbour.
	x, _, w, _ := merchantPriceRect(0, 0, merchantIconSize, merchantIconSize)
	if x < -(pitch-merchantIconSize)/2-1 || x+w > merchantIconSize+(pitch-merchantIconSize)/2+1 {
		t.Fatalf("price box [%d,%d) escapes its cell gap (icon %d, pitch %d)", x, x+w, merchantIconSize, pitch)
	}

	forEachUIFont(t, func(t *testing.T) {
		for _, label := range []string{
			"20000 g",                         // plain gold, biggest realistic buy price
			"5000 ap",                         // arena points
			"x3",                              // item currency alone
			"x3 +20000",                       // compound BEFORE compaction - must be clamped
			"x3 +" + compactCoinAmount(20000), // the shipped Scalewright form
			"sold out",
			"no value",
		} {
			fitted := merchantPriceLabel(label)
			if got := uiTextWidth(fitted); got > merchantPriceBoxW {
				t.Errorf("price %q renders %dpx wide, box is %dpx", fitted, got, merchantPriceBoxW)
			}
		}
	})

	// The shipped compound form must fit WITHOUT being clipped: a "20k" that
	// arrives as "20.." tells the player nothing.
	shipped := fmt.Sprintf("x%d +%s", 3, compactCoinAmount(20000))
	if merchantPriceLabel(shipped) != shipped {
		t.Fatalf("shipped compound price %q had to be clipped to %q", shipped, merchantPriceLabel(shipped))
	}
	if got, want := compactCoinAmount(20000), "20k"; got != want {
		t.Fatalf("compactCoinAmount(20000) = %q, want %q", got, want)
	}
	if got, want := compactCoinAmount(9999), "9999"; got != want {
		t.Fatalf("compactCoinAmount keeps small amounts exact: got %q, want %q", got, want)
	}
}

// Every shipped merchant's stock must fit the price box with its real currency
// wiring - the catalog is the thing that actually ships, so measure IT through
// the label the shop draws.
func TestShippedMerchantStockPricesFit(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	// No Merchant skill: the full, widest gold price.
	g := newTestGame(cfg, newTestWorld(cfg))
	var shops []*character.NPC
	for key := range character.NPCConfigInstance.NPCs {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatalf("NPC %q: %v", key, err)
		}
		if len(npc.MerchantStock) > 0 {
			shops = append(shops, npc)
		}
	}
	if len(shops) == 0 {
		t.Fatal("no merchant stock measured - the catalog did not load")
	}
	for _, npc := range shops {
		for _, entry := range npc.MerchantStock {
			label := g.merchantStockPriceText(npc, entry)
			if w := uiTextWidth(label); w > merchantPriceBoxW {
				t.Errorf("NPC %q sells %q at %q: %dpx wide, box is %dpx",
					npc.Key, entry.Item.Name, label, w, merchantPriceBoxW)
			}
		}
	}
}

// A quest card is sized from its OWN copy, and pages pack by height: wordy
// quests give fewer cards per page, terse ones more. Cards must never overlap
// or leave the list area.
func TestQuestCardsSizeToCopyAndPackByHeight(t *testing.T) {
	content := layoutRect{0, 0, tabbedMenuPanelW, 620}
	listTop, avail, pager := questCardListAvailable(content)
	maxRows := questCardMaxDescRowsFor(avail)
	cardW := pager.w

	short := questCardCopyFor("Short.", cardW, maxRows)
	long := questCardCopyFor(strings.Repeat("A wordy objective that wraps. ", 30), cardW, maxRows)
	if long.height <= short.height {
		t.Fatalf("long card (%d) must be taller than short card (%d)", long.height, short.height)
	}
	if !long.descClipped() && len(long.descLines) < maxRows {
		t.Fatalf("long copy neither filled the cap nor reported clipping: %d rows", len(long.descLines))
	}

	// A page of short cards must hold strictly more entries than a page of long
	// ones - that IS the "smart pagination" contract.
	manyShort := make([]questCardCopy, 12)
	manyLong := make([]questCardCopy, 12)
	for i := range manyShort {
		manyShort[i], manyLong[i] = short, long
	}
	shortPage := computeQuestContentLayout(content, manyShort, 0)
	longPage := computeQuestContentLayout(content, manyLong, 0)
	if len(shortPage.rows) <= len(longPage.rows) {
		t.Fatalf("short cards per page %d, long %d - pagination is not height-aware",
			len(shortPage.rows), len(longPage.rows))
	}

	// Geometry: no overlap, nothing past the pager.
	for _, layout := range []questContentLayout{shortPage, longPage} {
		for i, row := range layout.rows {
			if row.y < listTop {
				t.Errorf("card %d starts above the list top (%d < %d)", i, row.y, listTop)
			}
			if row.bottom() > pager.y {
				t.Errorf("card %d bottom %d runs into the pager at %d", i, row.bottom(), pager.y)
			}
			if i > 0 && row.y < layout.rows[i-1].bottom() {
				t.Errorf("card %d overlaps card %d", i, i-1)
			}
		}
	}

	// Paging covers every card exactly once, with no gaps.
	seen := 0
	for page := 0; page < longPage.totalPages; page++ {
		l := computeQuestContentLayout(content, manyLong, page)
		if l.pageStart != seen {
			t.Fatalf("page %d starts at %d, expected %d", page, l.pageStart, seen)
		}
		seen += len(l.rows)
	}
	if seen != len(manyLong) {
		t.Fatalf("pages covered %d of %d cards", seen, len(manyLong))
	}
}

// The clipped-copy tooltip is the promise that nothing authored is lost.
// Measured against the SHIPPED greetings in every font: a merchant greeting
// that overflows its two-line box must be detectable as clipped, with the
// whole text intact.
func TestShippedGreetingsThatOverflowStayRecoverable(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	// The generic dialog body reports the same condition through the layout.
	g := newTestGame(cfg, newTestWorld(cfg))
	long := strings.Repeat("Maruna turns a scale to the light and keeps talking. ", 40)
	npc := &character.NPC{Name: "Scalewright", DialogueData: &character.NPCDialogue{Greeting: long}}
	layout := g.dialogueLayout(npc, npcDialogWidth, npcDialogHeight)
	if !layout.bodyClipped() || len(layout.bodyFullLines) <= len(layout.bodyLines) {
		t.Fatal("an over-long body must report itself clipped and keep the full copy")
	}

	forEachUIFont(t, func(t *testing.T) {
		greetingW := computeNPCDialogSectionLayout(layoutRect{0, 0, npcDialogWidth, npcDialogHeight}, true).greeting.w
		overflowing := 0
		for key, npc := range character.NPCConfigInstance.NPCs {
			if npc.Dialogue == nil || npc.Dialogue.Greeting == "" {
				continue
			}
			full := wrapUIText(npc.Dialogue.Greeting, greetingW)
			shown := truncateWrappedLines(full, 2, greetingW)
			for _, line := range shown {
				if w := uiTextWidth(line); w > greetingW {
					t.Errorf("NPC %q greeting line %q is %dpx wide, box is %dpx", key, line, w, greetingW)
				}
			}
			if len(full) > len(shown) {
				overflowing++
				// The clipped view keeps its ellipsis cue and the full copy its tail.
				if !strings.HasSuffix(shown[len(shown)-1], "...") {
					t.Errorf("NPC %q greeting is clipped without an ellipsis cue: %q", key, shown[len(shown)-1])
				}
				if strings.Join(full, " ") == strings.Join(shown, " ") {
					t.Errorf("NPC %q full greeting was not preserved for the hover tooltip", key)
				}
			}
		}
		if overflowing == 0 {
			t.Skip("no shipped greeting overflows its two-line box in this font")
		}
	})
}

// Journal order is driven by what the player must DO next: hand-in first, then
// in progress, then closed - alphabetical inside each group.
func TestQuestJournalOrder(t *testing.T) {
	mk := func(name string, completed, claimed bool) *quests.Quest {
		return &quests.Quest{
			ID:             name,
			Definition:     &quests.QuestDefinition{Name: name},
			Completed:      completed,
			RewardsClaimed: claimed,
		}
	}
	doneB := mk("B done", true, true)
	activeA := mk("A active", false, false)
	turnInZ := mk("Z turn in", true, false)
	activeC := mk("C active", false, false)
	turnInA := mk("A turn in", true, false)
	doneA := mk("A done", true, true)

	list := []*quests.Quest{doneB, activeA, turnInZ, activeC, turnInA, doneA}
	sortQuestJournal(list)

	var got []string
	for _, q := range list {
		got = append(got, q.Definition.Name)
	}
	want := []string{"A turn in", "Z turn in", "A active", "C active", "A done", "B done"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("journal order:\n got %v\nwant %v", got, want)
	}

	// An auto-claimed quest (the victory quest) is closed, not waiting.
	auto := mk("Victory", true, true)
	auto.Definition.AutoClaim = true
	if questJournalRank(auto) != questRankDone {
		t.Fatalf("auto-claimed quest ranked %d, want done (%d)", questJournalRank(auto), questRankDone)
	}
}

// A paid NPC cast grants the buff for its AUTHORED span (not a mastery curve)
// and charges the gold.
func TestCastBuffServiceGrantsAuthoredDuration(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.gameLoop = &GameLoop{game: g}
	ih := &InputHandler{game: g}
	tps := cfg.GetTPS()

	g.party.Gold = 8000
	g.dialogNPC = &character.NPC{Name: "Apprentice Mira"}
	ih.handleCastBuff(&character.NPCDialogueChoice{
		Text: "Walk us over the water", Action: "cast_buff",
		Buff: "walk_on_water", DurationSeconds: 300, Cost: 2000,
	})
	if !g.walkOnWaterActive || g.walkOnWaterDuration != 300*tps {
		t.Fatalf("walk on water: active=%v duration=%d frames, want %d", g.walkOnWaterActive, g.walkOnWaterDuration, 300*tps)
	}
	if g.party.Gold != 6000 {
		t.Fatalf("gold after the 2000 cast = %d, want 6000", g.party.Gold)
	}
	if countCombatLog(g, "Apprentice Mira casts Walk on Water over the party for 5 min (-2000 gold).") != 1 {
		t.Fatal("paid cast did not log the NPC, canonical buff name, duration, and charged cost")
	}

	ih.handleCastBuff(&character.NPCDialogueChoice{
		Text: "Give us the deep breath", Action: "cast_buff",
		Buff: "water_breathing", DurationSeconds: 600, Cost: 5000,
	})
	if !g.waterBreathingActive || g.waterBreathingDuration != 600*tps {
		t.Fatalf("water breathing: active=%v duration=%d frames, want %d", g.waterBreathingActive, g.waterBreathingDuration, 600*tps)
	}
}

// Every refused paid cast leaves the purse and the buff exactly as they were
// and logs why. A chant already woven over the party refuses at ANY remaining
// span (an active-but-shorter buff used to be extended and charged, so stray
// clicks drained the purse), and so does a permanent walk-on-water card. A
// faded chant sells again.
func TestCastBuffServiceRefusals(t *testing.T) {
	cfg := loadTestConfig(t)
	tps := cfg.GetTPS()
	walk := &character.NPCDialogueChoice{
		Text: "Walk us over the water", Action: "cast_buff",
		Buff: "walk_on_water", DurationSeconds: 300, Cost: 2000,
	}
	waterCard := ""
	for key := range config.GlobalItems.Items {
		if def := cardDef(key); def != nil && def.CardWalkOnWater {
			waterCard = key
		}
	}
	if waterCard == "" {
		t.Fatal("fixture: no card grants walk-on-water")
	}
	covered := uitext.Text("dialog.is_already_woven_over_the_party_no", buffServiceLabel(walk.Buff))
	for _, tc := range []struct {
		name    string
		gold    int
		pose    func(*MMGame)
		choice  *character.NPCDialogueChoice
		wantLog string // "" = the cast is sold
	}{
		{"too poor", 100, func(*MMGame) {}, walk, uitext.Text("dialog.that_casting_costs_gold_your_purse_is", walk.Cost)},
		{"unknown buff", 9000, func(*MMGame) {},
			&character.NPCDialogueChoice{Text: "Nonsense", Action: "cast_buff", Buff: "not_a_buff", DurationSeconds: 60, Cost: 10},
			uitext.Text("dialog.nothing_happens")},
		{"chant active, nearly spent", 5000, func(g *MMGame) {
			g.walkOnWaterActive, g.walkOnWaterDuration = true, 10*tps
		}, walk, covered},
		{"permanent card", 5000, func(g *MMGame) { g.cardSlots[0].key = waterCard }, walk, covered},
		{"faded chant sells again", 5000, func(g *MMGame) {
			g.walkOnWaterActive, g.walkOnWaterDuration = false, 0
		}, walk, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorld(cfg))
			g.gameLoop = &GameLoop{game: g}
			g.party.Gold = tc.gold
			tc.pose(g)
			active, duration := g.walkOnWaterActive, g.walkOnWaterDuration
			(&InputHandler{game: g}).handleCastBuff(tc.choice)
			if tc.wantLog == "" {
				if !g.walkOnWaterActive || g.party.Gold != tc.gold-tc.choice.Cost {
					t.Fatalf("control failed: not re-sold (active=%v gold=%d)", g.walkOnWaterActive, g.party.Gold)
				}
				return
			}
			if g.party.Gold != tc.gold {
				t.Fatalf("a refused cast charged the party: gold = %d, want %d", g.party.Gold, tc.gold)
			}
			if g.walkOnWaterActive != active || g.walkOnWaterDuration != duration {
				t.Fatalf("a refused cast changed the buff: active %v->%v, duration %d->%d", active, g.walkOnWaterActive, duration, g.walkOnWaterDuration)
			}
			if countCombatLog(g, tc.wantLog) != 1 {
				t.Fatalf("the refusal did not log %q", tc.wantLog)
			}
		})
	}
}

func TestSwitchDialogTabClearsPendingBuffService(t *testing.T) {
	pending := &character.NPCDialogueChoice{
		Text: "Walk us over the water", Action: "cast_buff",
		Buff: "walk_on_water", DurationSeconds: 300, Cost: 2000,
	}
	g := &MMGame{
		dialogState: dialogState{
			dialogTab:            0,
			selectedChoice:       3,
			merchantBuyPage:      2,
			pendingBuffService:   pending,
			dialogLastClickedIdx: 4,
			dialogLastClickZone:  "service",
		},
	}

	g.switchDialogTab(1)

	if g.dialogTab != 1 || g.selectedChoice != 0 || g.merchantBuyPage != 0 {
		t.Fatalf("tab transition left stale selection state: tab=%d choice=%d page=%d",
			g.dialogTab, g.selectedChoice, g.merchantBuyPage)
	}
	if g.pendingBuffService != nil {
		t.Fatal("tab transition retained a deferred paid service")
	}
	if g.dialogLastClickedIdx != -1 || g.dialogLastClickZone != "" {
		t.Fatal("tab transition retained the previous tab's click tracker")
	}
}

func TestUtilityTimedBuffActivationUsesAuthoredFlags(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.gameLoop = &GameLoop{game: g}
	combat := NewCombatSystem(g)
	frames := 60 * cfg.GetTPS()

	// A canonical ID without its authored effect flag must not activate by name.
	if got := combat.activateUtilityTimedBuff(
		"walk_on_water",
		spells.SpellDefinition{},
		spells.UtilitySpellResult{Success: true},
		frames,
	); got != timedBuffNotHandled {
		t.Fatalf("flagless canonical spell activation = %d, want not handled", got)
	}
	if g.walkOnWaterActive {
		t.Fatal("canonical spell ID activated water walking without water_walk: true")
	}

	// An alternate spell ID with the authored flag must activate the shared
	// water-walking runtime effect.
	if got := combat.activateUtilityTimedBuff(
		"alternate_water_stride",
		spells.SpellDefinition{},
		spells.UtilitySpellResult{Success: true, WaterWalk: true},
		frames,
	); got != timedBuffApplied {
		t.Fatalf("authored water-walk activation = %d, want applied", got)
	}
	if !g.walkOnWaterActive || g.walkOnWaterDuration != frames {
		t.Fatalf("authored water_walk flag did not control runtime state: active=%v duration=%d",
			g.walkOnWaterActive, g.walkOnWaterDuration)
	}
}

func TestCastBuffServiceRunsTimedBuffActivationHooks(t *testing.T) {
	cases := []struct {
		id        string
		isActive  func(*MMGame) bool
		getRadius func(*MMGame) float64
		authored  func(*config.SpellDefinitionConfig) float64
	}{
		{
			id:        "torch_light",
			isActive:  func(g *MMGame) bool { return g.torchLightActive },
			getRadius: func(g *MMGame) float64 { return g.torchLightRadius },
			authored:  func(d *config.SpellDefinitionConfig) float64 { return d.LightRadiusTiles },
		},
		{
			id:        "wizard_eye",
			isActive:  func(g *MMGame) bool { return g.wizardEyeActive },
			getRadius: func(g *MMGame) float64 { return g.wizardEyeRadiusTiles },
			authored:  func(d *config.SpellDefinitionConfig) float64 { return d.RadarRadiusTiles },
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			cfg := loadTestConfig(t)
			g := newTestGame(cfg, newTestWorld(cfg))
			g.gameLoop = &GameLoop{game: g}
			g.party.Gold = 100
			ih := &InputHandler{game: g}
			def, ok := config.GetSpellDefinition(tc.id)
			if !ok {
				t.Fatalf("spell %q missing", tc.id)
			}

			ih.handleCastBuff(&character.NPCDialogueChoice{
				Text: tc.id, Action: "cast_buff", Buff: tc.id,
				DurationSeconds: 60, Cost: 10,
			})

			if !tc.isActive(g) {
				t.Fatalf("%s service did not activate its timed buff", tc.id)
			}
			if got := tc.getRadius(g); got != tc.authored(def) {
				t.Fatalf("%s radius = %v, want authored %v", tc.id, got, tc.authored(def))
			}
			if g.party.Gold != 90 {
				t.Fatalf("%s service left %d gold, want 90", tc.id, g.party.Gold)
			}
		})
	}
}

// Mira's dialog is a TABBED service: paid casts live as icon rows on their own
// tab (with icons that exist), and never leak into the Talk tab's choice list.
func TestBuffServiceDialogTabsAndGeometry(t *testing.T) {
	cfg := loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	data := character.NPCConfigInstance.NPCs["mtrader0"]
	if data == nil {
		t.Fatal("mtrader0 missing from the catalog")
	}
	npc := &character.NPC{Name: data.Name, DialogueData: data.Dialogue}
	g := newTestGame(cfg, newTestWorld(cfg))

	if got := g.npcDialogKindFor(npc); got != dialogKindBuffService {
		t.Fatalf("Mira resolves to dialog kind %d, want dialogKindBuffService (%d)", got, dialogKindBuffService)
	}
	services := buffServiceChoices(npc)
	if len(services) == 0 {
		t.Fatal("Mira offers no service rows")
	}
	for _, service := range services {
		if strings.Contains(strings.ToLower(service.Text), "gold") {
			t.Errorf("service label %q duplicates its authored cost", service.Text)
		}
		if label := g.dialogueChoiceLabel(service); !strings.Contains(label, fmt.Sprintf("%d gold", service.Cost)) {
			t.Errorf("generic dialogue label %q does not derive cost %d", label, service.Cost)
		}
	}
	if !g.npcDialogHasTalkTab(npc) {
		t.Fatal("Mira still has quest dialogue, so the Talk tab must exist")
	}

	// The service rows must not also appear as text choices on the Talk tab.
	for _, c := range g.visibleNPCChoices(npc) {
		if c.Action == "cast_buff" {
			t.Errorf("cast_buff %q leaked into the choice list", c.Text)
		}
	}

	// Geometry: rows fit between greeting and footer, and never overlap.
	dialog := npcDialogLayout(g)
	layout := computeNPCDialogSectionLayout(layoutRect(dialog), true)
	maxRows := buffServiceMaxRows(dialog.x, dialog.y, dialog.w, dialog.h)
	if len(services) > maxRows {
		t.Fatalf("%d service rows authored but only %d fit the dialog", len(services), maxRows)
	}
	var prevBottom int
	for i := range services {
		x, y, w, h := buffServiceRowRect(dialog.x, dialog.y, dialog.w, i)
		if y < layout.greeting.bottom() {
			t.Errorf("row %d starts inside the greeting block", i)
		}
		if y+h > layout.footer[0].y {
			t.Errorf("row %d bottom %d runs into the footer at %d", i, y+h, layout.footer[0].y)
		}
		if x < dialog.x || x+w > dialog.x+dialog.w {
			t.Errorf("row %d escapes the dialog horizontally", i)
		}
		if i > 0 && y < prevBottom {
			t.Errorf("row %d overlaps row %d", i, i-1)
		}
		prevBottom = y + h
	}

	// Every offered buff must have a real party buff id and a label.
	for _, c := range services {
		if !g.isTimedBuffID(c.Buff) {
			t.Errorf("service %q names unknown buff %q", c.Text, c.Buff)
		}
		if buffServiceLabel(c.Buff) == c.Buff {
			t.Errorf("buff %q has no spells.yaml display name for the row/tooltip", c.Buff)
		}
	}
}

func TestValidateNPCCastBuffsRejectsCatalogBeyondDialogCapacity(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	maxRows := buffServiceMaxRows(0, 0, npcDialogWidth, npcDialogHeight)

	makeChoices := func(count int) []*character.NPCDialogueChoice {
		choices := make([]*character.NPCDialogueChoice, count)
		for i := range choices {
			choices[i] = &character.NPCDialogueChoice{
				Text: "Water charm", Action: "cast_buff",
				Buff: "walk_on_water", DurationSeconds: 60, Cost: 1,
			}
		}
		return choices
	}

	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"full_service": {Dialogue: &character.NPCDialogue{Choices: makeChoices(maxRows)}},
	}}
	if err := g.validateNPCCastBuffs(); err != nil {
		t.Fatalf("%d service rows should fit capacity %d: %v", maxRows, maxRows, err)
	}

	choices := makeChoices(maxRows + 1)
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"overfull_service": {Dialogue: &character.NPCDialogue{Choices: choices}},
	}}

	err := g.validateNPCCastBuffs()
	if err == nil {
		t.Fatalf("%d service rows passed validation with capacity %d", len(choices), maxRows)
	}
	want := fmt.Sprintf("%d cast_buff service rows exceed dialog capacity %d", len(choices), maxRows)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("capacity error = %q, want it to contain %q", err, want)
	}
}

// Buff-service rows follow the dialog list convention: the first click only
// selects, the second within the window queues the purchase.
func TestBuffServiceRowNeedsDoubleClick(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.dialogActive = true
	g.dialogNPC = &character.NPC{
		Name: "Apprentice Mira",
		DialogueData: &character.NPCDialogue{
			Greeting: "Charms for coin.",
			Choices: []*character.NPCDialogueChoice{{
				Text: "Walk us over the water", Action: "cast_buff",
				Buff: "walk_on_water", DurationSeconds: 300, Cost: 100,
			}},
		},
	}
	g.party.Gold = 1000

	dlg := npcDialogLayout(g)
	x, y, w, h := buffServiceRowRect(dlg.x, dlg.y, dlg.w, 0)
	rowClick := func() {
		g.mouseLeftClicks = []queuedClick{{x: x + w/2, y: y + h/2, at: 1000}}
		ui.drawBuffServiceDialog(ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()), dlg.x, dlg.y, dlg.w, dlg.h)
	}

	rowClick()
	if g.pendingBuffService != nil {
		t.Fatal("a single click queued the paid cast - gold is one misclick away again")
	}
	rowClick()
	if g.pendingBuffService == nil {
		t.Fatal("control failed: the double click did not queue the cast")
	}
}
