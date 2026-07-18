package monster

import "testing"

func TestCurrentAIBehaviorPrecedence(t *testing.T) {
	foe := &Monster3D{HitPoints: 1}

	cases := []struct {
		name string
		mob  Monster3D
		want AIBehaviorMode
	}{
		{
			name: "inert wins over every dynamic state",
			mob: Monster3D{
				HitPoints: 1, BossDormant: true, Pacified: true, Bound: true,
				AIFoe: foe, BossAggro: true, State: StateFleeing,
			},
			want: AIBehaviorInert,
		},
		{
			name: "bound ally wins over a malformed charm and cached foe",
			mob:  Monster3D{HitPoints: 1, Bound: true, Pacified: true, AIFoe: foe, BossAggro: true},
			want: AIBehaviorBoundAlly,
		},
		{
			name: "charm wins over redirection",
			mob:  Monster3D{HitPoints: 1, Pacified: true, AIFoe: foe, BossAggro: true},
			want: AIBehaviorPacified,
		},
		{
			name: "foe wins over relentless party pursuit",
			mob:  Monster3D{HitPoints: 1, AIFoe: foe, BossAggro: true, State: StateFleeing},
			want: AIBehaviorFightFoe,
		},
		{
			name: "relentless party pursuit wins over flee",
			mob:  Monster3D{HitPoints: 1, BossAggro: true, State: StateFleeing},
			want: AIBehaviorRelentlessParty,
		},
		{
			name: "flee wins over passive idle",
			mob:  Monster3D{HitPoints: 1, PassiveUntilAttacked: true, State: StateFleeing},
			want: AIBehaviorFleeing,
		},
		{
			name: "unhit passive monster stays passive",
			mob:  Monster3D{HitPoints: 1, PassiveUntilAttacked: true, State: StateIdle},
			want: AIBehaviorPassive,
		},
		{
			name: "hit passive monster resumes ordinary party behavior",
			mob:  Monster3D{HitPoints: 1, PassiveUntilAttacked: true, WasAttacked: true, State: StateIdle},
			want: AIBehaviorSeekParty,
		},
		{
			name: "ordinary monster seeks party",
			mob:  Monster3D{HitPoints: 1, State: StatePatrolling},
			want: AIBehaviorSeekParty,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mob.CurrentAIBehavior(); got != tc.want {
				t.Fatalf("CurrentAIBehavior() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCalmForSocialBehavior(t *testing.T) {
	foe := &Monster3D{HitPoints: 1}
	cases := []struct {
		name string
		mob  Monster3D
		want bool
	}{
		{name: "ordinary idle", mob: Monster3D{HitPoints: 1, State: StateIdle}, want: true},
		{name: "ordinary patrol", mob: Monster3D{HitPoints: 1, State: StatePatrolling}, want: true},
		{name: "passive idle", mob: Monster3D{HitPoints: 1, State: StateIdle, PassiveUntilAttacked: true}, want: true},
		{name: "bound ally", mob: Monster3D{HitPoints: 1, State: StateIdle, Bound: true}, want: false},
		{name: "pacified monster", mob: Monster3D{HitPoints: 1, State: StateIdle, Pacified: true}, want: false},
		{name: "redirected foe fight", mob: Monster3D{HitPoints: 1, State: StateIdle, AIFoe: foe}, want: false},
		{name: "fleeing monster", mob: Monster3D{HitPoints: 1, State: StateFleeing}, want: false},
		{name: "engaged monster", mob: Monster3D{HitPoints: 1, State: StateIdle, IsEngagingPlayer: true}, want: false},
		{name: "hit monster", mob: Monster3D{HitPoints: 1, State: StateIdle, WasAttacked: true}, want: false},
		{name: "inert set piece", mob: Monster3D{HitPoints: 1, State: StateIdle, BossDormant: true}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mob.IsCalmForSocialBehavior(); got != tc.want {
				t.Fatalf("IsCalmForSocialBehavior() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPartyTargetPolicy(t *testing.T) {
	foe := &Monster3D{HitPoints: 1}
	cases := []struct {
		name string
		mob  Monster3D
		want bool
	}{
		{name: "ordinary idle", mob: Monster3D{HitPoints: 1, State: StateIdle}, want: false},
		{name: "sight engagement", mob: Monster3D{HitPoints: 1, IsEngagingPlayer: true, State: StateAlert}, want: true},
		{name: "direct hit", mob: Monster3D{HitPoints: 1, WasAttacked: true, State: StateIdle}, want: true},
		{name: "relentless hunter", mob: Monster3D{HitPoints: 1, BossAggro: true, State: StateIdle}, want: true},
		{name: "fleeing hit mob", mob: Monster3D{HitPoints: 1, WasAttacked: true, State: StateFleeing}, want: false},
		{name: "passive stale engagement", mob: Monster3D{HitPoints: 1, PassiveUntilAttacked: true, IsEngagingPlayer: true}, want: false},
		{name: "bound ally", mob: Monster3D{HitPoints: 1, Bound: true, IsEngagingPlayer: true}, want: false},
		{name: "pacified monster", mob: Monster3D{HitPoints: 1, Pacified: true, IsEngagingPlayer: true}, want: false},
		{name: "redirected enemy", mob: Monster3D{HitPoints: 1, AIFoe: foe, IsEngagingPlayer: true}, want: false},
		{name: "inert set piece", mob: Monster3D{HitPoints: 1, BossDormant: true, WasAttacked: true}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mob.TargetsParty(); got != tc.want {
				t.Fatalf("TargetsParty() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestActiveCombatPolicy(t *testing.T) {
	foe := &Monster3D{HitPoints: 1}
	cases := []struct {
		name string
		mob  Monster3D
		want bool
	}{
		{name: "calm idle", mob: Monster3D{HitPoints: 1, State: StateIdle}, want: false},
		{name: "party fight", mob: Monster3D{HitPoints: 1, IsEngagingPlayer: true, State: StateAlert}, want: true},
		{name: "sticky party hostility", mob: Monster3D{HitPoints: 1, WasAttacked: true, State: StateIdle}, want: true},
		{name: "crossfire fight", mob: Monster3D{HitPoints: 1, AIFoe: foe, State: StatePursuing}, want: true},
		{name: "bound follower", mob: Monster3D{HitPoints: 1, Bound: true, State: StatePursuing}, want: false},
		{name: "bound crossfire", mob: Monster3D{HitPoints: 1, Bound: true, AIFoe: foe, State: StatePursuing}, want: true},
		{name: "pacified", mob: Monster3D{HitPoints: 1, Pacified: true, IsEngagingPlayer: true}, want: false},
		{name: "fleeing", mob: Monster3D{HitPoints: 1, WasAttacked: true, State: StateFleeing}, want: false},
		{name: "inert", mob: Monster3D{HitPoints: 1, BossDormant: true, WasAttacked: true}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mob.IsInCombat(); got != tc.want {
				t.Fatalf("IsInCombat() = %v, want %v", got, tc.want)
			}
		})
	}
}
