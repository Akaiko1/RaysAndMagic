package game

import (
	"fmt"
	"sort"

	"ugataima/internal/quests"
)

// Quest banners: the producer that turns journal changes into screen banners
// (the channel lives in screen_banner.go). A DIFF WATCHER, not notification
// calls - every event is derived by comparing the manager's state against last
// frame's snapshot, so a new progress source raises banners with no call of its
// own and none can be forgotten.

// questBannerSnapshot is the per-quest state the diff compares.
type questBannerSnapshot struct {
	status quests.QuestStatus
	// peak is the highest count ever seen, and the only counter compared against:
	// exterminate counters are census-derived and move BACKWARDS on a respawn, so
	// a re-kill would replay counters the player already watched go past.
	peak    int
	claimed bool
	// seenGen stamps the pass that last saw this quest, so quests the manager
	// dropped are forgotten without allocating a presence set every frame.
	seenGen uint64
}

// questBannerEvent is one banner-worthy change found by a diff pass.
type questBannerEvent struct {
	quest *quests.Quest
	kind  screenBannerKind
}

// questBannerText is this producer's wording. The counter comes from the quest
// itself so it can never disagree with the journal.
func questBannerText(kind screenBannerKind, q *quests.Quest) string {
	name := q.Definition.Name
	switch kind {
	case bannerQuestTaken:
		return "New quest - " + name
	case bannerQuestProgress:
		return fmt.Sprintf("%s  %d/%d", name, q.CurrentCount, q.Target())
	case bannerQuestDone:
		return "Quest complete - " + name
	default:
		return "Reward claimed - " + name
	}
}

// questBannerEventFor turns "what this quest was" and "what it is now" into a
// banner, or into nothing.
//
// ORDER IS LOAD-BEARING: a payout wins over the completion it arrives with (an
// auto-claimed quest gets ONE banner), and a completion wins over the counter
// jump that MarkCompleted causes.
func questBannerEventFor(q *quests.Quest, was questBannerSnapshot, known bool) (screenBannerKind, bool) {
	if !known {
		// Usually "just taken" - but a kill quest whose targets are already dead is
		// credited inside handleGiveQuest, so a first sighting can arrive completed
		// or paid. Announce what it IS, or accepting raises nothing.
		return firstSightingBanner(q)
	}
	switch {
	case q.RewardsClaimed && !was.claimed:
		return bannerQuestPaid, true
	case q.Status == quests.QuestStatusCompleted && was.status != quests.QuestStatusCompleted:
		return bannerQuestDone, true
	case q.CurrentCount > was.peak && q.Status == quests.QuestStatusActive:
		return bannerQuestProgress, true
	default:
		return bannerQuestTaken, false
	}
}

// firstSightingBanner picks the banner for a quest the watcher has never seen
// before, from what it ALREADY is. ok is false for a sighting worth no banner -
// a definition the manager is merely holding, in no state to announce.
func firstSightingBanner(q *quests.Quest) (screenBannerKind, bool) {
	switch {
	case q.RewardsClaimed:
		return bannerQuestPaid, true
	case q.Status == quests.QuestStatusCompleted:
		return bannerQuestDone, true
	case q.Status == quests.QuestStatusActive:
		return bannerQuestTaken, true
	default:
		return bannerQuestTaken, false
	}
}

// syncQuestBanners is the one diff pass. announce=false adopts the state
// silently (boot, new run, save load). A quiet frame allocates nothing - the
// journal is walked in place and only events are collected; this runs 60x/s.
func (g *MMGame) syncQuestBanners(announce bool) {
	if g.questManager == nil {
		return
	}
	if g.questBannerSeen == nil {
		g.questBannerSeen = make(map[string]questBannerSnapshot)
		announce = false // first pass ever: there is nothing to compare against
	}
	g.questBannerGen++
	gen := g.questBannerGen

	var events []questBannerEvent
	stamped := 0
	g.questManager.EachQuest(func(q *quests.Quest) {
		if q == nil || q.Definition == nil {
			return
		}
		was, known := g.questBannerSeen[q.ID]
		stamped++
		g.questBannerSeen[q.ID] = questBannerSnapshot{
			status:  q.Status,
			peak:    max(was.peak, q.CurrentCount),
			claimed: q.RewardsClaimed,
			seenGen: gen,
		}
		if !announce {
			return
		}
		if kind, ok := questBannerEventFor(q, was, known); ok {
			events = append(events, questBannerEvent{quest: q, kind: kind})
		}
	})
	// EachQuest walks a map, so the order is imposed on what was collected - by
	// the one quest order (questIDLess).
	sort.Slice(events, func(i, j int) bool { return questIDLess(events[i].quest, events[j].quest) })
	for _, e := range events {
		g.queueQuestBanner(e.kind, e.quest)
	}

	// Forget quests this pass did not see (repeatables dropped at nightfall), so
	// the next time one is taken it reads as new again. The map can only hold more
	// than the pass stamped when something was dropped, so the common frame skips
	// the sweep entirely.
	if len(g.questBannerSeen) > stamped {
		for id, snap := range g.questBannerSeen {
			if snap.seenGen != gen {
				delete(g.questBannerSeen, id)
			}
		}
	}
}

// resyncQuestBannerBaseline adopts the current quest state without banners.
func (g *MMGame) resyncQuestBannerBaseline() {
	g.questBannerSeen = nil
	g.syncQuestBanners(false)
}

func (g *MMGame) queueQuestBanner(kind screenBannerKind, q *quests.Quest) {
	g.queueBanner(kind, questBannerText(kind, q))
}
