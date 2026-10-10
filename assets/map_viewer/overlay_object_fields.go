package main

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
)

// The object inspector shows every authored npcs.yaml field, grouped as the
// game uses it and only for the object types where the game reads it.

var overlayPresentations = []string{"npc", "scenery", "wall_mounted", "landmark", "wide_landmark", "invisible"}

func overlayPresentationHelp() string {
	return "npc: a person who turns to face the party. scenery: a prop that slowly spins in place. wall_mounted: hangs on the nearest wall. landmark: a tall solid monument. wide_landmark: a flat facade spanning several tiles. invisible: no sprite, only an interaction point."
}

// overlayPresentationDefaults keeps size, sprite and facade fields valid for the
// chosen presentation, as the game's visual-size check requires.
func overlayPresentationDefaults(n *character.NPCData, category string) {
	switch category {
	case "npc":
		n.SizeClass = config.SizeClassPerson
	case "invisible":
		n.SizeClass, n.Sprite = "", ""
	default:
		if n.SizeClass == "" || n.SizeClass == config.SizeClassPerson {
			n.SizeClass = "full_tile"
		}
	}
	if category == "wide_landmark" {
		if n.GridSpanTiles < 2 {
			n.GridSpanTiles, n.GridSpanDir = 2, "e"
		}
		n.NoSpin = false
	} else {
		n.GridSpanTiles, n.GridSpanDir, n.WallBacked, n.CrystalShimmer = 0, "", false, nil
	}
}

func (v *viewer) overlaySprites() []string {
	sprites := []string{}
	for _, def := range character.NPCConfigInstance.NPCs {
		if def != nil && def.Sprite != "" && !slices.Contains(sprites, def.Sprite) {
			sprites = append(sprites, def.Sprite)
		}
	}
	sort.Strings(sprites)
	return sprites
}

// overlayShopDisplays lists the shop display art the catalog already uses:
// names known to exist, since the game's boot check panics on a missing sprite.
func (v *viewer) overlayShopDisplays() []string {
	displays := []string{""}
	for _, def := range character.NPCConfigInstance.NPCs {
		if def != nil && def.ShopDisplay != "" && !slices.Contains(displays, def.ShopDisplay) {
			displays = append(displays, def.ShopDisplay)
		}
	}
	sort.Strings(displays)
	return displays
}

func (v *viewer) overlayBiomes() []string {
	biomes := []string{}
	for _, m := range v.maps {
		if m.Config != nil && m.Config.Biome != "" && !slices.Contains(biomes, m.Config.Biome) {
			biomes = append(biomes, m.Config.Biome)
		}
	}
	sort.Strings(biomes)
	return biomes
}

func overlayItemNames(weapons bool) []string {
	names := []string{}
	if weapons && config.GlobalWeapons != nil {
		for _, w := range config.GlobalWeapons.Weapons {
			if w != nil && w.Name != "" {
				names = append(names, w.Name)
			}
		}
	} else if !weapons && config.GlobalItems != nil {
		for _, it := range config.GlobalItems.Items {
			if it != nil && it.Name != "" {
				names = append(names, it.Name)
			}
		}
	}
	sort.Strings(names)
	return slices.Compact(names)
}

// overlayDoorKeys lists only items the game accepts as door keys (door_key > 0).
func overlayDoorKeys() []string {
	keys := []string{}
	if config.GlobalItems != nil {
		for key, it := range config.GlobalItems.Items {
			if it != nil && it.DoorKey > 0 {
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func overlayWeaponRarities() []string {
	out := []string{""}
	if config.GlobalWeapons != nil {
		for _, w := range config.GlobalWeapons.Weapons {
			if w != nil && w.Rarity != "" && !slices.Contains(out, w.Rarity) {
				out = append(out, w.Rarity)
			}
		}
	}
	sort.Strings(out[1:])
	return out
}

// overlayEncounterMonsters matches the Maps palette: fish never fight and
// champion carriers only work through the arena duel flow.
func (v *viewer) overlayEncounterMonsters() []string {
	keys := []string{}
	for k, m := range v.monsterCfg.Monsters {
		if m.Disposition != monster.DispositionFish && m.Champion == "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// overlayBecomeGuild leaves a valid guild after a type switch. The guild owns its
// ledger and membership-gated portal, so ordinary stock, spells, choices and the
// Town Portal anchor go; missing terms, art and greeting come from an authored guild.
func overlayBecomeGuild(n *character.NPCData) {
	n.RenderCategory, n.NoSpin, n.TownPortal, n.HideWhenVisited = "landmark", true, false, false
	n.Inventory, n.StockWeaponsRarity, n.SellAvailable, n.Spells = nil, "", false, nil
	n.ShopDialogue, n.ShopDisplay = false, ""
	template := overlayAuthoredGuild()
	if n.Guild == nil {
		n.Guild = &character.GuildServiceConfig{}
		if template != nil {
			*n.Guild = *template.Guild
		}
	}
	if n.Dialogue == nil {
		n.Dialogue = &character.NPCDialogue{}
	}
	n.Dialogue.Choices = nil
	if n.Dialogue.Greeting == "" && template != nil && template.Dialogue != nil {
		n.Dialogue.Greeting = template.Dialogue.Greeting
	}
	if n.PropModel != nil {
		n.PropModel.Shape = "thieves_guild"
		n.GridSpanTiles, n.GridSpanDir = 0, ""
	}
}

// overlayAuthoredGuild is the first catalog guild by key, or nil.
func overlayAuthoredGuild() *character.NPCData {
	if character.NPCConfigInstance == nil {
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(character.NPCConfigInstance.NPCs)) {
		if n := character.NPCConfigInstance.NPCs[key]; n != nil && n.Type == character.NPCTypeThievesGuild && n.Guild != nil {
			return n
		}
	}
	return nil
}

func (v *viewer) overlayObjectFields(d *overlayDocument) []overlayField {
	p := &v.overlay
	if p.selected < 0 || p.selected >= len(d.state.Data.NPCSpawns) {
		return nil
	}
	spawn := d.state.Data.NPCSpawns[p.selected]
	key := spawn.NPCKey
	n := overlayClone(d.npc(key))
	if n == nil {
		return nil
	}
	// Actions edit the draft directly; field edits commit the private copy below.
	edit := func(fn func(x *character.NPCData)) func() {
		return func() { d.checkpoint(); fn(d.editNPC(key)) }
	}
	var fields []overlayField
	add := func(fs ...overlayField) { fields = append(fields, fs...) }
	nested := func(f overlayField) overlayField { f.depth = 1; return f }
	quests := append([]string{""}, v.overlayCatalog("quests", "quests")...)
	items := v.overlayCatalog("items", "items")
	opensDialog := n.Type != character.NPCTypeLootCrate && n.Type != character.NPCTypeSpellLectern

	add(overlayString("Name", &n.Name, "Shown in the interaction prompt and the dialog title."),
		overlayInfo("Position", fmt.Sprintf("Column %d, row %d", spawn.X, spawn.Y), "Use Move object to place it elsewhere."),
		overlayAction("Move object", "Click empty walkable ground.", func() { p.tool = "object-move" }))

	// Identity.
	typeField := overlayString("Object type", &n.Type, "What using the object does. Crates and spell books never open a dialog; doors use the Door section.", character.NPCTypeOrder...)
	applyType := typeField.apply
	typeField.apply = func(s string) error {
		was := n.Type
		if err := applyType(s); err != nil {
			return err
		}
		if s == character.NPCTypeDoor {
			n.RenderCategory, n.SizeClass, n.GridSpanTiles, n.GridSpanDir = "door", "full_tile", 0, ""
			if n.DoorBehavior == "" {
				n.DoorBehavior = character.NPCDoorBehaviorLocked
			}
		} else if was == character.NPCTypeDoor {
			n.RenderCategory = "scenery"
		}
		if s != character.NPCTypeEncounter {
			n.Encounter = nil
		}
		if s == character.NPCTypeThievesGuild {
			overlayBecomeGuild(n)
		} else {
			n.Guild = nil
			// The guild model is guild-only, and other types offer no 3D model
			// fields to remove it by hand.
			if was == character.NPCTypeThievesGuild && n.PropModel != nil && n.PropModel.Shape == "thieves_guild" {
				n.PropModel = nil
			}
		}
		return nil
	}
	add(overlayHeading("Identity"),
		overlayInfo("Definition key", key, "Placements may share one definition. Make local copy gives this placement its own."),
		typeField,
		overlayAction("Make local copy", "Copies this definition for this placement only. Catalog crates must keep their key.", func() {
			if n.Type == character.NPCTypeLootCrate {
				d.err = "Loot crates use their catalog key. Choose another crate from the library."
				return
			}
			d.checkpoint()
			newKey := d.newObjectKey()
			d.state.NPCs[newKey] = overlayClone(n)
			d.state.NPCs[newKey].EditorOwnerMap = d.key
			if e := d.state.NPCs[newKey].Encounter; e != nil && e.QuestID != "" {
				e.QuestID = newKey + "_encounter"
			}
			d.state.Data.NPCSpawns[p.selected].NPCKey = newKey
			v.overlayRemoveUnusedLocalNPC(d, key)
		}),
		overlayString("Palette description", &n.Description, "Shown only in the Maps palette tooltip."),
		overlayList("Palette biomes", &n.Biomes, "Biomes whose Maps palette offers this object. Empty offers it everywhere.", v.overlayBiomes()))

	// Appearance.
	add(overlayHeading("Appearance"))
	if n.Type == character.NPCTypeDoor {
		add(overlayInfo("Presentation", "door", "Doors always stand across their doorway."))
	} else {
		f := overlayString("Presentation", &n.RenderCategory, overlayPresentationHelp(), overlayPresentations...)
		apply := f.apply
		f.apply = func(s string) error {
			if err := apply(s); err != nil {
				return err
			}
			overlayPresentationDefaults(n, s)
			return nil
		}
		add(f)
	}
	if n.RenderCategory != "invisible" {
		sprites := v.overlaySprites()
		add(overlayString("Sprite", &n.Sprite, "Art shown in the world.", sprites...),
			overlayString("Sprite after use", &n.VisitedSprite, "Optional art once the object has been used, for example an emptied barrel.", append([]string{""}, sprites...)...))
		sizes := []string{}
		for size := range v.cfg.Graphics.SizeClasses {
			if n.RenderCategory == "npc" && size == config.SizeClassPerson || n.RenderCategory != "npc" && config.IsPropSizeClass(size) {
				sizes = append(sizes, size)
			}
		}
		sort.Strings(sizes)
		add(overlayString("Size", &n.SizeClass, "People are always person-sized; props use a prop size. full_tile is exactly one tile high.", sizes...))
	}
	tiles := append([]string{""}, slices.Sorted(maps.Keys(v.tileDataByKey))...)
	add(overlayString("Ground under the object", &n.GroundTile, "Terrain drawn under this object wherever it is placed. Empty keeps the map's ground.", tiles...))
	if (n.RenderCategory == "scenery" || n.RenderCategory == "landmark") && n.PropModel == nil {
		add(overlayBool("Keep still (no spin)", &n.NoSpin, "Scenery and landmark sprites slowly turn around their own axis. Turn this on to keep the object at its fixed angle. People always turn to face the party."))
	}
	if n.RenderCategory == "wide_landmark" {
		dir := map[string]string{"e": "east", "s": "south"}[n.GridSpanDir]
		dirField := overlayString("Facade direction", &dir, "The facade spans from this tile toward the east or the south.", "east", "south")
		applyDir := dirField.apply
		dirField.apply = func(s string) error {
			if err := applyDir(s); err != nil {
				return err
			}
			n.GridSpanDir = map[string]string{"east": "e", "south": "s"}[s]
			return nil
		}
		add(overlayInt("Facade width (tiles)", &n.GridSpanTiles, "2 to 4 tiles."), dirField,
			overlayBool("Backed by a wall", &n.WallBacked, "Draw the facade flush against the wall behind it."))
		if s := n.CrystalShimmer; s == nil {
			add(overlayAction("Add crystal shimmer", "Animated glints on crystals in the facade art.", edit(func(x *character.NPCData) {
				x.CrystalShimmer = &character.CrystalShimmer{IntervalSeconds: 6, DurationSeconds: 1.5, Crystals: []character.FacadeCrystal{{Center: [2]float64{0.5, 0.5}, Radius: [2]float64{0.1, 0.1}, Color: [3]int{200, 230, 255}}}}
			})))
		} else {
			add(overlayFloat("Shimmer interval (seconds)", &s.IntervalSeconds, "Time between glints."), overlayFloat("Shimmer duration (seconds)", &s.DurationSeconds, "Length of one glint; at most the interval."))
			for i := range s.Crystals {
				c := &s.Crystals[i]
				add(nested(overlayNumbers(fmt.Sprintf("Crystal %d centre (x, y)", i+1), []*float64{&c.Center[0], &c.Center[1]}, "Position in the sprite, 0 to 1 on each axis.", 0, 1)),
					nested(overlayNumbers(fmt.Sprintf("Crystal %d radius (x, y)", i+1), []*float64{&c.Radius[0], &c.Radius[1]}, "Size in the sprite, 0 to 1; must stay inside the art.", 0, 1)),
					nested(overlayRGB(fmt.Sprintf("Crystal %d colour (r, g, b)", i+1), &c.Color, "Glint colour, 0 to 255 per channel.")),
					nested(overlayAction(fmt.Sprintf("Remove crystal %d", i+1), "Remove this glint area.", edit(func(x *character.NPCData) {
						x.CrystalShimmer.Crystals = slices.Delete(x.CrystalShimmer.Crystals, i, i+1)
					}))))
			}
			add(overlayAction("Add crystal", "Add another glint area.", edit(func(x *character.NPCData) {
				x.CrystalShimmer.Crystals = append(x.CrystalShimmer.Crystals, character.FacadeCrystal{Center: [2]float64{0.5, 0.5}, Radius: [2]float64{0.1, 0.1}, Color: [3]int{200, 230, 255}})
			})), overlayAction("Remove crystal shimmer", "Remove the glint animation.", edit(func(x *character.NPCData) { x.CrystalShimmer = nil })))
		}
	}
	if n.Type == character.NPCTypeLootCrate && n.RenderCategory == "scenery" || n.Type == character.NPCTypeThievesGuild {
		if m := n.PropModel; m == nil {
			add(overlayAction("Add 3D model", "Draw the crate as a lit 3D model instead of a flat sprite.", edit(func(x *character.NPCData) {
				x.PropModel = &character.NPCPropModel{Shape: "crates", Body: [3]int{133, 88, 48}, Trim: [3]int{80, 79, 75}, UseSeconds: 1.2}
				if x.Type == character.NPCTypeThievesGuild {
					x.PropModel.Shape = "thieves_guild"
				}
				x.HideWhenVisited = false
			})))
		} else {
			shapes := []string{"crates", "campfire"}
			if n.Type == character.NPCTypeThievesGuild {
				shapes = []string{"thieves_guild"}
			}
			add(overlayHeading("3D model"),
				overlayString("Model shape", &m.Shape, "Built-in model.", shapes...),
				overlaySignedFloat("Rotation (degrees)", &m.YawDegrees, "Turns the model around its vertical axis.", -360, 360),
				overlayRGB("Body colour (r, g, b)", &m.Body, "Main colour, 0 to 255 per channel."),
				overlayRGB("Trim colour (r, g, b)", &m.Trim, "Accent colour, 0 to 255 per channel."),
				overlayFloat("Use time (seconds)", &m.UseSeconds, "How long using the object takes. Must be positive."),
				overlayFloat("Light radius (tiles)", &m.LightRadiusTiles, "0 for no light. Light radius and strength are set together."),
				overlayFloat("Light strength (0 to 1)", &m.LightIntensity, "0 for no light."),
				overlayAction("Remove 3D model", "Draw the flat sprite again.", edit(func(x *character.NPCData) { x.PropModel = nil })))
		}
	}
	if n.Type == character.NPCTypeEncounter {
		add(overlayBool("Steam after use", &n.SteamWhenVisited, "Show a pale steam plume once the object has been used, like a closed valve."),
			overlayBool("Transparent (activity prop)", &n.Transparent, "Required for quest activity scenery. No other effect."))
	}

	// Availability.
	add(overlayHeading("Availability"),
		overlayInt("Minimum party level", &n.MinPartyLevel, "Hidden until the party reaches this level. 0: no requirement."),
		overlayBool("Night only", &n.NightOnly, "Present only at night."))
	if n.PropModel == nil {
		add(overlayBool("Hide after use", &n.HideWhenVisited, "Disappears once used. Use only when its action records a visit."))
	}
	add(overlayString("Requires completed quest", &n.RequiresQuest, "The object's services stay closed until this quest is done.", quests...),
		overlayBool("Refuses a Lich", &n.RejectsLich, "Will not speak to a party that includes a Lich."))
	if n.Type == character.NPCTypeThievesGuild {
		add(overlayInfo("Town Portal", "Unlocked with membership", "Guild entrances appear separately from ordinary region destinations."))
	} else {
		add(overlayBool("Town Portal arrival point", &n.TownPortal, "Town Portal brings the party to this object in its region."))
	}

	// Dialogue.
	if opensDialog {
		if n.Dialogue == nil {
			n.Dialogue = &character.NPCDialogue{}
		}
		g := n.Dialogue
		add(overlayHeading("Dialogue"),
			overlayString("Greeting", &g.Greeting, "Text shown when the dialog opens."),
			overlayString("Question before actions", &g.ChoicePrompt, "Optional line above the action list, for example 'What do you do?'."),
			overlayString("Message after use", &g.VisitedMessage, "Shown instead of the greeting once the object has been used."),
			overlayString("Message while a quest is active", &g.ActiveMessage, "Shown while a quest from this object is in progress."),
			overlayString("Message after the quest", &g.CompletedMessage, "Shown once its quest is completed."),
			overlayString("Greeting while services are locked", &g.QuestGreeting, "Shown while Requires completed quest is not yet met."),
			overlayAction("+ Add dialogue action", "Choose what an interaction does. Each action shows its own settings.", func() { v.overlayAddChoice(d, nil) }))
		for i, c := range g.Choices {
			if c == nil {
				continue
			}
			path := []int{i}
			add(nested(overlayNavigate(c.Text, "Action: "+c.Action, "Edit this dialogue action and its settings.", func() { v.overlaySelect("choice", p.selected); p.choicePath = path })))
		}
		add(overlayHeading("Quest messages"))
		ids := make([]string, 0, len(g.QuestMessages))
		for id := range g.QuestMessages {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			msg := g.QuestMessages[id]
			set := func(label string, field *string, help string) overlayField {
				f := overlayString(id+": "+label, field, help)
				apply := f.apply
				f.apply = func(s string) error {
					if err := apply(s); err != nil {
						return err
					}
					g.QuestMessages[id] = msg
					return nil
				}
				return nested(f)
			}
			add(set("offer", &msg.Offer, "Shown when the quest is offered."),
				set("accepted", &msg.Accepted, "Shown when the party accepts it."),
				set("in progress", &msg.Active, "Shown while it is in progress."),
				set("not finished", &msg.NotCompleted, "Shown when turning in too early."),
				set("already active", &msg.AlreadyActive, "Shown when offering it again while active."),
				set("completed", &msg.Completed, "Shown after it is completed."),
				set("ineligible", &msg.Ineligible, "Shown when the party cannot take it yet."),
				set("refuses undead", &msg.RejectedUndead, "Shown when the quest refuses an undead party."),
				nested(overlayAction("Remove messages for "+id, "Use the default quest texts.", edit(func(x *character.NPCData) { delete(x.Dialogue.QuestMessages, id) }))))
		}
		add(overlayAction("+ Add quest messages", "Custom texts for one quest this object offers or takes in.", func() {
			v.overlayChoose("Quest messages", "Choose the quest.", quests[1:], func(id string) error {
				d.checkpoint()
				x := d.editNPC(key)
				if x.Dialogue.QuestMessages == nil {
					x.Dialogue.QuestMessages = map[string]character.NPCQuestMessages{}
				}
				x.Dialogue.QuestMessages[id] = x.Dialogue.QuestMessages[id]
				return nil
			})
		}))
	}

	// Services.
	if s := n.Guild; s != nil {
		add(overlayHeading("Guild membership"),
			overlayInt("One-time gold fee", &s.MembershipGold, "Paid once by the party."),
			overlayInt("Doors opened without keys", &s.NonKeyDoorsOpened, "Successful force or lockpick openings required before joining. Keys do not count."),
			overlayString("Merchant miniature", &s.Art.Merchant, "Guild report art."),
			overlayString("Town miniature", &s.Art.Town, "Guild location art."),
			overlayString("Dungeon miniature", &s.Art.Dungeon, "Guild report art."),
			overlayString("Creature miniature", &s.Art.Creature, "Guild report art."),
			overlayString("Wildlife miniature", &s.Art.Wildlife, "Guild report art."),
			overlayString("Caravan miniature", &s.Art.Caravan, "Guild report art."))
	}
	stocked := len(n.Inventory) > 0 || n.StockWeaponsRarity != "" || n.SellAvailable
	// The game's own dialog dispatch decides whether a shop here is ever drawn.
	shopDrawn, shopTabs, dialog := game.NPCShopOffer(key, n)
	if n.Type == character.NPCTypeMerchant || stocked {
		add(overlayHeading("Shop"))
		if !shopDrawn {
			add(overlayInfo("Shop is hidden", "Opens the "+dialog+" dialog", "That dialog has no shop. Remove the stock, or the service that takes priority."))
		} else if shopTabs && !n.ShopDialogue {
			add(overlayInfo("Shop is hidden", "Dialogue actions open first", "Turn on Shop together with dialogue."))
		}
		add(
			overlayString("Currency", &n.Currency, "Empty: gold. arena_points: the arena victory currency. item:<key>: prices are counts of that item.", append([]string{"", "arena_points"}, overlayPrefixed("item:", items)...)...),
			overlayString("Shop display art", &n.ShopDisplay, "Decorative art over the shop's top rail. Empty: none.", v.overlayShopDisplays()...),
			overlayBool("Buys from the party", &n.SellAvailable, "The party can sell items here. Gold shops only."),
			overlayInt("Restock every (weeks)", &n.StockRefreshWeeks, "0 keeps stock until sold out."),
			overlayBool("Shop together with dialogue", &n.ShopDialogue, "Required when the object also has dialogue actions; otherwise the actions hide the shop."),
			overlayBool("Arena champions board", &n.ArenaBoard, "Adds the champions' leaderboard tab, as the arena gladiators have."),
			overlayString("Also sell every weapon of rarity", &n.StockWeaponsRarity, "Adds all weapons of this rarity at the price below.", overlayWeaponRarities()...),
			overlayInt("Price of those weapons", &n.StockWeaponsCost, "Required with a rarity."))
		for i, it := range n.Inventory {
			if it == nil {
				continue
			}
			label := fmt.Sprintf("Stock %d", i+1)
			add(nested(overlayString(label+" kind", &it.Type, "weapon looks the name up among weapons; anything else among items.", "item", "potion", "weapon")),
				nested(overlayString(label+" name", &it.Name, "Display name of the item or weapon.", overlayItemNames(strings.EqualFold(it.Type, "weapon"))...)),
				nested(overlayInt(label+" price", &it.Cost, "0 uses the item's own value.")),
				nested(overlayInt(label+" quantity", &it.Quantity, "-1 never sells out; 0 means one copy.")),
				nested(overlayString(label+" tab", &it.Tab, "Optional shop tab. Use tabs for every entry or none.")),
				nested(overlayString(label+" paid in item", &it.CurrencyItem, "Optional item key used as the price instead of the shop currency.", append([]string{""}, items...)...)),
				nested(overlayInt(label+" extra gold", &it.GoldCost, "Gold surcharge on top of an item price.")),
				nested(overlayAction("Remove "+strings.ToLower(label), "Remove this stock entry.", edit(func(x *character.NPCData) { x.Inventory = slices.Delete(x.Inventory, i, i+1) }))))
		}
		add(overlayAction("+ Add stock", "Add an item to the shop.", edit(func(x *character.NPCData) {
			x.Inventory = append(x.Inventory, &character.NPCItem{Type: "item", Quantity: 1})
		})))
	} else if opensDialog && n.Type != character.NPCTypeDoor && shopDrawn {
		add(overlayAction("+ Add shop", "Let this object sell items.", edit(func(x *character.NPCData) {
			x.Inventory = append(x.Inventory, &character.NPCItem{Type: "item", Quantity: 1})
			x.ShopDialogue = shopTabs
		})))
	} else if opensDialog && n.Type != character.NPCTypeDoor {
		add(overlayInfo("Shop", "Not available", "This object opens the "+dialog+" dialog, which has no shop."))
	}
	if n.Type == character.NPCTypeSpellTrader {
		add(overlayHeading("Spells for sale"))
		ids := make([]string, 0, len(n.Spells))
		for id := range n.Spells {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			sp := n.Spells[id]
			if sp == nil {
				continue
			}
			add(nested(overlayInt(id+" price", &sp.Cost, "Gold price. Name and description come from spells.yaml.")),
				nested(overlayAction("Remove "+id, "Stop selling this spell.", edit(func(x *character.NPCData) { delete(x.Spells, id) }))))
		}
		add(overlayAction("+ Add spell", "Sell another spell.", func() {
			v.overlayChoose("Spell for sale", "Choose a spell.", v.overlayCatalog("spells", "spells"), func(id string) error {
				d.checkpoint()
				x := d.editNPC(key)
				if x.Spells == nil {
					x.Spells = map[string]*character.NPCSpell{}
				}
				x.Spells[id] = &character.NPCSpell{Cost: 500}
				return nil
			})
		}))
		if g := n.Dialogue; g != nil {
			add(overlayString("Not enough gold", &g.InsufficientGold, "Shown when the party cannot pay."),
				overlayString("Spell already known", &g.AlreadyKnown, "Shown when the buyer already knows the spell."),
				overlayString("Purchase done", &g.Success, "Shown after a purchase."))
		}
	}
	if n.Type == character.NPCTypeSkillTrainer {
		add(overlayHeading("Training"))
		for _, tier := range []string{"expert", "master", "grandmaster"} {
			cost := n.Training[tier]
			f := overlayInt(titleCase(tier)+" training price", &cost, "Gold price to reach this mastery. 0: not offered.")
			apply := f.apply
			f.apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				if n.Training == nil {
					n.Training = map[string]int{}
				}
				if cost <= 0 {
					delete(n.Training, tier)
				} else {
					n.Training[tier] = cost
				}
				return nil
			}
			add(f)
		}
	}
	if n.Type == character.NPCTypeDoor {
		add(overlayHeading("Door"), overlayString("Door kind", &n.DoorBehavior, "locked: opens with a key or a stat check. champion_portcullis: opens after the arena duel.", character.NPCDoorBehaviorLocked, character.NPCDoorBehaviorChampionPortcullis))
		if n.DoorBehavior == character.NPCDoorBehaviorLocked {
			add(overlayString("Lock word", &n.LockLabel, "One adjective used in door messages: 'The <word> door will not budge'. Empty uses 'locked'."),
				overlayList("Opening keys", &n.DoorKeyItemKeys, "Items that unlock the door. Only items marked door_key are offered.", overlayDoorKeys()))
			for i := range n.DoorStatReqs {
				r := &n.DoorStatReqs[i]
				add(nested(overlayString(fmt.Sprintf("Check %d stat", i+1), &r.Stat, "A party member with enough of this stat can force the door.", "Might", "Intellect")),
					nested(overlayInt(fmt.Sprintf("Check %d value", i+1), &r.Value, "Required stat value.")),
					nested(overlayAction(fmt.Sprintf("Remove check %d", i+1), "Remove this stat check.", edit(func(x *character.NPCData) { x.DoorStatReqs = slices.Delete(x.DoorStatReqs, i, i+1) }))))
			}
			add(overlayAction("+ Add stat check", "Let a strong or clever hero force the door.", edit(func(x *character.NPCData) {
				x.DoorStatReqs = append(x.DoorStatReqs, character.NPCDoorStatReq{Stat: "Might", Value: 30})
			})))
		}
	}
	if n.Type == character.NPCTypeSpellLectern {
		if n.Lectern == nil {
			add(overlayHeading("Spell book"), overlayAction("Set up the spell book", "Choose which spell this book teaches.", edit(func(x *character.NPCData) { x.Lectern = &character.NPCLectern{} })))
		} else {
			add(overlayHeading("Spell book"), overlayString("Teaches spell", &n.Lectern.Spell, "A fixed spell. Leave empty to use the random pool.", append([]string{""}, v.overlayCatalog("spells", "spells")...)...))
			if n.Lectern.Spell == "" {
				add(overlayList("Random spell pool", &n.Lectern.Pool, "One spell from this list is taught at random.", v.overlayCatalog("spells", "spells")))
			}
		}
	}
	if n.Type == character.NPCTypeEncounter {
		add(overlayHeading("Combat encounter"))
		add(v.overlayEncounterFields(d, n, edit)...)
		add(overlayHeading("Statue summons"))
		monsters := v.overlayEncounterMonsters()
		for i, s := range n.Summons {
			if s == nil {
				continue
			}
			label := fmt.Sprintf("Summon %d", i+1)
			add(nested(overlayString(label+" statuette", &s.Statuette, "Item placed on the statue.", items...)),
				nested(overlayString(label+" monster", &s.Monster, "Creature that answers.", monsters...)),
				nested(overlayString(label+" choice text", &s.Label, "Dialog choice for this summon.")),
				nested(overlayString(label+" quest", &s.QuestID, "Optional quest that must be active.", quests...)),
				nested(overlayString(label+" locked reply", &s.LockedResponse, "Shown while the quest condition is not met.")),
				nested(overlayAction("Remove "+strings.ToLower(label), "Remove this summon.", edit(func(x *character.NPCData) { x.Summons = slices.Delete(x.Summons, i, i+1) }))))
		}
		add(overlayAction("+ Add summon", "Placing a statuette on this object summons a creature.", edit(func(x *character.NPCData) {
			x.Summons = append(x.Summons, &character.NPCSummon{Label: "Place the statuette."})
		})))
	}
	// Field reads stay pure: commit the private copy only after a successful edit.
	for i := range fields {
		if apply := fields[i].apply; apply != nil {
			fields[i].apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				d.state.NPCs[key] = n
				return nil
			}
		}
	}
	return fields
}

func (v *viewer) overlayEncounterFields(d *overlayDocument, n *character.NPCData, edit func(func(*character.NPCData)) func()) []overlayField {
	key := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
	if n.Encounter == nil {
		return []overlayField{overlayAction("Add combat encounter", "Configure monsters and rewards, then add a Fight action.", edit(func(x *character.NPCData) {
			x.Encounter = &character.NPCEncounter{Type: "combat", QuestID: key + "_encounter", FirstVisitOnly: true, Rewards: &monster.EncounterRewards{}}
		}))}
	}
	e := n.Encounter
	nested := func(f overlayField) overlayField { f.depth = 1; return f }
	out := []overlayField{overlayBool("Offer only once", &e.FirstVisitOnly, "The fight is offered once: the object counts as used as soon as the fight starts, win or lose."), overlayString("Quest log ID", &e.QuestID, "Unique save identity for this fight."), overlayString("Quest log title", &e.QuestName, "Name in the quest log."), overlayString("Quest log description", &e.QuestDescription, "Objective text in the quest log."), overlayString("Fight announcement", &e.StartMessage, "Message when the fight begins.")}
	keys := v.overlayEncounterMonsters()
	for i, m := range e.Monsters {
		if m == nil {
			continue
		}
		out = append(out, overlayString(fmt.Sprintf("Group %d monster", i+1), &m.Type, "Monster to spawn.", keys...), overlayInt(fmt.Sprintf("Group %d minimum", i+1), &m.CountMin, "Minimum count."), overlayInt(fmt.Sprintf("Group %d maximum", i+1), &m.CountMax, "Maximum count."), overlayAction(fmt.Sprintf("Remove group %d", i+1), "Remove this monster group.", edit(func(x *character.NPCData) {
			x.Encounter.Monsters = slices.Delete(x.Encounter.Monsters, i, i+1)
		})))
	}
	out = append(out, overlayAction("Add monster group", "Adds one monster; choose its kind and count.", edit(func(x *character.NPCData) {
		x.Encounter.Monsters = append(x.Encounter.Monsters, &character.EncounterMonster{Type: keys[0], CountMin: 1, CountMax: 1})
	})))
	if e.Rewards == nil {
		e.Rewards = &monster.EncounterRewards{}
	}
	r := e.Rewards
	out = append(out, overlayHeading("Victory rewards"), overlayInt("Gold", &r.Gold, "Gold awarded on victory."), overlayInt("Experience", &r.Experience, "Experience awarded on victory."), overlayString("Victory message", &r.CompletionMessage, "Shown after every monster of the fight is defeated."), overlayBool("Frees captured heroes", &r.FreesCaptives, "On victory, imprisoned heroes join the reserve roster."))
	chests := []*monster.TreasureChestReward{}
	if r.TreasureChest != nil {
		chests = append(chests, r.TreasureChest)
	}
	for i := range r.TreasureChests {
		chests = append(chests, &r.TreasureChests[i])
	}
	tables := append([]string{""}, v.overlayCatalog("loots", "loot_tables")...)
	items := v.overlayCatalog("items", "items")
	weapons := v.overlayCatalog("weapons", "weapons")
	maps := []string{""}
	for _, m := range v.maps {
		maps = append(maps, m.Key)
	}
	for i, c := range chests {
		label := fmt.Sprintf("Chest %d", i+1)
		out = append(out, nested(overlayString(label+" ID", &c.ID, "Unique save identity for the chest.")),
			nested(overlayString(label+" map", &c.Map, "Empty: this map.", maps...)),
			nested(overlayInt(label+" column", &c.TileX, "Tile where the chest appears.")),
			nested(overlayInt(label+" row", &c.TileY, "Tile where the chest appears.")),
			nested(overlayString(label+" sprite", &c.Sprite, "Optional chest art.", append([]string{""}, v.overlaySprites()...)...)),
			nested(overlayFloat(label+" size (tiles)", &c.SizeTiles, "0 uses the default chest size.")),
			nested(overlayInt(label+" gold", &c.Gold, "Gold inside.")),
			nested(overlayList(label+" items", &c.Items, "Item keys inside.", items)),
			nested(overlayList(label+" weapons", &c.Weapons, "Weapon keys inside.", weapons)),
			nested(overlayInt(label+" random weapons", &c.RandomWeaponCount, "Extra random weapons.")),
			nested(overlayString(label+" loot table", &c.LootTable, "Optional loot table rolled into the chest.", tables...)),
			nested(overlayString(label+" message", &c.CompletionMessage, "Shown when the chest appears.")),
			nested(overlayAction("Remove "+strings.ToLower(label), "Remove this chest reward.", edit(func(x *character.NPCData) {
				rw := x.Encounter.Rewards
				if rw.TreasureChest != nil {
					if i == 0 {
						rw.TreasureChest = nil
						return
					}
					i--
				}
				rw.TreasureChests = slices.Delete(rw.TreasureChests, i, i+1)
			}))))
	}
	out = append(out, overlayAction("+ Add reward chest", "A chest appears when the fight is won.", edit(func(x *character.NPCData) {
		if x.Encounter.Rewards == nil {
			x.Encounter.Rewards = &monster.EncounterRewards{}
		}
		x.Encounter.Rewards.TreasureChests = append(x.Encounter.Rewards.TreasureChests, monster.TreasureChestReward{ID: v.overlayFreeChestID(d, key)})
	})))
	return out
}

// overlayFreeChestID names a new reward chest after its object, skipping every
// chest ID the game knows: chests share one world-wide list keyed by ID.
func (v *viewer) overlayFreeChestID(d *overlayDocument, key string) string {
	npcs := map[string]*character.NPCData{}
	if character.NPCConfigInstance != nil {
		maps.Copy(npcs, character.NPCConfigInstance.NPCs)
	}
	maps.Copy(npcs, d.state.NPCs)
	configs := map[string]*config.MapConfig{}
	for _, m := range v.maps {
		configs[m.Key] = m.Config
	}
	used := game.RewardChestIDs(npcs, configs)
	id := key + "_chest"
	for i := 2; used[id]; i++ {
		id = fmt.Sprintf("%s_chest_%d", key, i)
	}
	return id
}

func overlayPrefixed(prefix string, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = prefix + k
	}
	return out
}
