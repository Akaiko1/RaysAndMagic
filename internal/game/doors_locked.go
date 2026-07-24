package game

import (
	"fmt"
	"math/rand"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
)

// Locked doors (type "door", door_behavior "locked") bar a passage until the
// party unlocks them with a matching key or forces them open with a stat check.
// Unlike a champion portcullis (the other explicit door behavior), a locked
// door has its OWN persisted open state: npc.Visited. Once opened it stays open
// across save/load - the same free persistence chests and dragon statues use.
//
// Unlock rules are authored per door (npcs.yaml): DoorKeyItemKeys (consumable
// items.yaml keys that fit), DoorStatReqs (a member whose effective stat meets
// the threshold can force it). The Skeleton Key (master_key attribute) opens
// ANY locked door and is never consumed.

// doorStatGetters is the single source for door force-check stats. Validation
// uses its keys and the live unlock path uses its accessors, so a supported YAML
// stat cannot be accepted at load yet turn into a dead option at runtime.
var doorStatGetters = map[string]func(*character.MMCharacter) int{
	"might":     (*character.MMCharacter).GetEffectiveMight,
	"intellect": (*character.MMCharacter).GetEffectiveIntellect,
}

// ValidateDoorNPCs fail-fast validates the complete door contract. Door
// behavior belongs to type "door" while pose belongs to render_category "door";
// requiring both directions prevents a new editor prop from silently behaving
// as a lock or an arena gate.
func ValidateDoorNPCs(npcs map[string]*character.NPCData) error {
	for key, npc := range npcs {
		if npc == nil {
			continue
		}
		isTypeDoor := npc.Type == character.NPCTypeDoor
		isRenderDoor := npc.RenderCategory == "door"
		if isTypeDoor != isRenderDoor {
			return fmt.Errorf("door NPC %q must use both type door and render_category door (got type %q, render_category %q)", key, npc.Type, npc.RenderCategory)
		}
		if !isTypeDoor {
			continue
		}
		switch npc.DoorBehavior {
		case character.NPCDoorBehaviorLocked:
			if npc.Dialogue == nil {
				return fmt.Errorf("locked door NPC %q needs dialogue for its unlock choices", key)
			}
			for _, req := range npc.DoorStatReqs {
				if _, ok := doorStatGetters[strings.ToLower(req.Stat)]; !ok {
					return fmt.Errorf("door NPC %q force stat %q is not a door stat (valid: Might, Intellect)", key, req.Stat)
				}
				if req.Value <= 0 {
					return fmt.Errorf("door NPC %q force stat %q needs value > 0", key, req.Stat)
				}
			}
			if len(npc.DoorKeyItemKeys) == 0 && len(npc.DoorStatReqs) == 0 {
				return fmt.Errorf("locked door NPC %q has no key items and no stat reqs - only the Skeleton Key could open it", key)
			}
			seenKeys := make(map[string]bool, len(npc.DoorKeyItemKeys))
			for _, itemKey := range npc.DoorKeyItemKeys {
				if seenKeys[itemKey] {
					return fmt.Errorf("locked door NPC %q lists key item %q more than once", key, itemKey)
				}
				seenKeys[itemKey] = true
				def, ok := config.GetItemDefinition(itemKey)
				if !ok || def == nil {
					return fmt.Errorf("locked door NPC %q key item %q is not a known items.yaml key", key, itemKey)
				}
				if def.DoorKey <= 0 {
					return fmt.Errorf("locked door NPC %q key item %q lacks door_key", key, itemKey)
				}
			}
		case character.NPCDoorBehaviorChampionPortcullis:
			if npc.LockLabel != "" || len(npc.DoorKeyItemKeys) > 0 || len(npc.DoorStatReqs) > 0 {
				return fmt.Errorf("champion portcullis NPC %q cannot define lock requirements", key)
			}
		default:
			return fmt.Errorf("door NPC %q has unknown door_behavior %q (valid: %s, %s)", key, npc.DoorBehavior, character.NPCDoorBehaviorLocked, character.NPCDoorBehaviorChampionPortcullis)
		}
	}
	return nil
}

// lockedDoorClosed reports whether a locked door is still shut (barring the way
// and offering its unlock dialog). An opened door (Visited) is invisible and
// walkable.
func lockedDoorClosed(npc *character.NPC) bool {
	return character.IsLockedDoor(npc) && !npc.Visited
}

// registerMapStaticCollision restores every authored, non-monster collision
// block for the current map. It is deliberately called only after NPC Visited
// flags have been restored, so an opened locked door cannot become an invisible
// wall after loading a save.
func (g *MMGame) registerMapStaticCollision() {
	g.registerBuildingFootprints()
	g.registerLockedDoors()
}

// registerLockedDoors makes every still-closed locked door on the current map
// solid: one static entity per door tile. Runs on map arrival AFTER the door
// NPCs' Visited state is restored, so an already-opened door never re-bars.
func (g *MMGame) registerLockedDoors() {
	if g.collisionSystem == nil || g.world == nil {
		return
	}
	if g.lockedDoorEntityIDs == nil {
		g.lockedDoorEntityIDs = make(map[string]bool)
	}
	ts := float64(g.config.GetTileSize())
	for _, npc := range g.world.NPCs {
		if !lockedDoorClosed(npc) {
			continue
		}
		id := lockedDoorEntityID(npc)
		if g.lockedDoorEntityIDs[id] {
			continue
		}
		g.lockedDoorEntityIDs[id] = true
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, npc.X, npc.Y, ts*0.9, ts*0.9, collision.CollisionTypeNPC, true))
	}
}

func lockedDoorEntityID(npc *character.NPC) string {
	return fmt.Sprintf("lockeddoor:%.0f:%.0f", npc.X, npc.Y)
}

// clearLockedDoorEntities unregisters every locked-door collision entity (map
// switch / save load rebuild).
func (g *MMGame) clearLockedDoorEntities() {
	for id := range g.lockedDoorEntityIDs {
		if g.collisionSystem != nil {
			g.collisionSystem.UnregisterEntity(id)
		}
		delete(g.lockedDoorEntityIDs, id)
	}
}

// doorUnlockOption is one available way to open a specific door this instant,
// materialized into a dialogue choice. kind is the sole behavior discriminator;
// the other fields carry the selected method's data.
type doorUnlockOption struct {
	text          string
	keyItemKey    string // consumable items.yaml key (master/forcing leave this empty)
	masterKeyName string // held master key: never consumed
	kind          doorUnlockKind
	chancePct     int
	actorName     string
}

type doorUnlockKind int

const (
	doorUnlockMasterKey doorUnlockKind = iota
	doorUnlockConsumableKey
	doorUnlockForce
	doorUnlockLockpick
)

// partyMasterKeyName reports the held master key. The item attribute, not a
// hard-coded display name, is the source of truth for that capability.
func (g *MMGame) partyMasterKeyName() (string, bool) {
	for _, it := range g.party.Inventory {
		if it.Attributes["master_key"] > 0 {
			return it.Name, true
		}
	}
	return "", false
}

// availableDoorUnlocks enumerates every unlock the party can perform on this
// door RIGHT NOW: the master key (if held), each authored key the bag holds,
// the best living lockpicker, then each stat requirement some living member
// meets. A jammed lock returns only key methods.
func (g *MMGame) availableDoorUnlocks(npc *character.NPC) []doorUnlockOption {
	var opts []doorUnlockOption
	if masterKeyName, ok := g.partyMasterKeyName(); ok {
		opts = append(opts, doorUnlockOption{
			text:          fmt.Sprintf("Open it with the %s", masterKeyName),
			masterKeyName: masterKeyName,
			kind:          doorUnlockMasterKey,
		})
	}
	for _, itemKey := range npc.DoorKeyItemKeys {
		def, ok := config.GetItemDefinition(itemKey)
		if !ok || def == nil || def.DoorKey <= 0 {
			continue // load validation rejects this; keep a malformed runtime NPC inert
		}
		if g.party.CountItemsByName(def.Name) > 0 {
			opts = append(opts, doorUnlockOption{
				text:       fmt.Sprintf("Unlock it with the %s", def.Name),
				keyItemKey: itemKey,
				kind:       doorUnlockConsumableKey,
			})
		}
	}
	if npc.DoorLockBroken {
		return opts
	}
	var bestLockpicker *character.MMCharacter
	bestChance := 0
	for _, m := range g.party.Members {
		if m == nil || m.HitPoints <= 0 || !m.HasSkill(character.SkillLockpicking) {
			continue
		}
		chance := character.LockpickingChancePct(m.SkillTier(character.SkillLockpicking))
		if chance > bestChance {
			bestLockpicker, bestChance = m, chance
		}
	}
	if bestLockpicker != nil {
		opts = append(opts, doorUnlockOption{
			text: fmt.Sprintf("Pick the lock (%s, %d%%)", bestLockpicker.Name, bestChance),
			kind: doorUnlockLockpick, chancePct: bestChance, actorName: bestLockpicker.Name,
		})
	}
	for _, req := range npc.DoorStatReqs {
		getter := doorStatGetters[strings.ToLower(req.Stat)]
		if getter == nil {
			continue // load validation rejects this; keep a malformed runtime NPC inert
		}
		for _, m := range g.party.Members {
			if m != nil && m.HitPoints > 0 && getter(m) >= req.Value {
				opts = append(opts, doorUnlockOption{
					text: fmt.Sprintf("Force it open (%s, %s %d, %d%%)", m.Name, req.Stat, req.Value, character.DoorForceChancePct),
					kind: doorUnlockForce, chancePct: character.DoorForceChancePct, actorName: m.Name,
				})
				break // one forcing option per requirement, named for the first qualifying party member
			}
		}
	}
	return opts
}

// lockedDoorChoices materializes the locked door's unlock options into the
// generic dialogue choice shape. This is intentionally derived on demand rather
// than written into NPC.DialogueData: DialogueData points at shared YAML data,
// while the available options depend on this party's live inventory and stats.
func (g *MMGame) lockedDoorChoices(npc *character.NPC) []*character.NPCDialogueChoice {
	opts := g.availableDoorUnlocks(npc)
	choices := make([]*character.NPCDialogueChoice, 0, len(opts)+1)
	for i, o := range opts {
		choices = append(choices, &character.NPCDialogueChoice{
			Text:               o.text,
			Action:             "open_door",
			RuntimeOptionIndex: i,
		})
	}
	choices = append(choices, &character.NPCDialogueChoice{Text: "Step back", Action: "leave"})
	return choices
}

// lockedDoorGreeting returns the dialogue greeting for a door: the authored one
// when at least one unlock is available, or a sealed-shut message when the party
// can do nothing to it yet (so the player learns the door exists and is stuck).
func (g *MMGame) lockedDoorGreeting(npc *character.NPC, hasOptions bool) string {
	if npc.DoorLockBroken {
		if hasOptions {
			return "The lock is jammed beyond finesse or force. A key can still turn it."
		}
		return "The lock is jammed. Only a fitting key can open this door now."
	}
	if hasOptions {
		if npc.DialogueData != nil && npc.DialogueData.Greeting != "" {
			return npc.DialogueData.Greeting
		}
		return "The lock is within your means."
	}
	label := npc.LockLabel
	if label == "" {
		label = "locked"
	}
	return fmt.Sprintf("The %s door will not budge. You have no key that fits and no way to force it.", label)
}

// openLockedDoor resolves a chosen unlock option: consumes a single-use key
// (master key and non-key attempts consume nothing), marks the door open
// (Visited, persisted), and drops its collision block so the party can pass.
func (g *MMGame) openLockedDoor(npc *character.NPC, optIdx int) {
	if !lockedDoorClosed(npc) {
		return
	}
	opts := g.availableDoorUnlocks(npc)
	if optIdx < 0 || optIdx >= len(opts) {
		return
	}
	opt := opts[optIdx]
	opened := false
	switch opt.kind {
	case doorUnlockMasterKey:
		g.AddCombatMessage(fmt.Sprintf("The %s turns without resistance - the %s door swings open.", opt.masterKeyName, doorLabelOrDefault(npc)))
		opened = true
	case doorUnlockConsumableKey:
		def, ok := config.GetItemDefinition(opt.keyItemKey)
		if !ok || def == nil || !g.party.RemoveItemsByName(def.Name, 1) {
			return // key vanished between opening the dialog and confirming
		}
		g.AddCombatMessage(fmt.Sprintf("The %s turns in the lock and breaks off - the door opens.", def.Name))
		opened = true
	case doorUnlockForce, doorUnlockLockpick:
		opened, _ = resolveNonKeyDoorAttempt(npc, opt.chancePct, rand.Intn(100))
		if opened {
			verb := "forces"
			if opt.kind == doorUnlockLockpick {
				verb = "picks"
			}
			g.AddCombatMessage(fmt.Sprintf("%s %s the %s door open.", opt.actorName, verb, doorLabelOrDefault(npc)))
		} else if npc.DoorLockBroken {
			g.AddCombatMessage("The third failed attempt jams the lock. Only a key can open it now.")
		} else {
			g.AddCombatMessage(fmt.Sprintf("The attempt fails. %d non-key attempt(s) remain before the lock jams.",
				character.DoorMaxNonKeyAttempts-npc.DoorAttempts))
		}
	default:
		return
	}
	if !opened {
		g.dialogActive = false
		g.dialogNPC = nil
		return
	}
	npc.Visited = true
	if g.collisionSystem != nil {
		g.collisionSystem.UnregisterEntity(lockedDoorEntityID(npc))
	}
	delete(g.lockedDoorEntityIDs, lockedDoorEntityID(npc))
	g.dialogActive = false
	g.dialogNPC = nil
}

// resolveNonKeyDoorAttempt is the deterministic lock state transition shared
// by Might, Intellect, Lockpicking, and tests. roll is [0,99].
func resolveNonKeyDoorAttempt(npc *character.NPC, chancePct, roll int) (opened, jammed bool) {
	if npc == nil || npc.DoorLockBroken {
		return false, npc != nil && npc.DoorLockBroken
	}
	npc.DoorAttempts++
	if roll < chancePct {
		return true, false
	}
	if npc.DoorAttempts >= character.DoorMaxNonKeyAttempts {
		npc.DoorLockBroken = true
	}
	return false, npc.DoorLockBroken
}

func doorLabelOrDefault(npc *character.NPC) string {
	if npc.LockLabel != "" {
		return npc.LockLabel
	}
	return "heavy"
}
