package game

const respawnDaySaveVersion = 1

// migrateRespawnDays operates on the restore copy, never the caller's snapshot.
// Legacy respawn stamps were phase+1 (zero means unstamped). Respawns now
// use the same one-based calendar as the HUD and shops. Arena stamps stay phases.
func migrateRespawnDays(save *GameSave) {
	if save.RespawnDayVersion >= respawnDaySaveVersion {
		return
	}
	if save.MapRespawnDay != nil {
		days := make(map[string]int, len(save.MapRespawnDay))
		for key, stamp := range save.MapRespawnDay {
			if stamp > 0 {
				days[key] = calendarDayFromPhase(stamp - 1)
			}
		}
		save.MapRespawnDay = days
	}
	save.RespawnDayVersion = respawnDaySaveVersion
}
