package character

// ValidateNPCDefinitions uses the loader's exact rules without replacing the
// active catalog. Callers supply private drafts because spell backfill edits it.
func ValidateNPCDefinitions(defs map[string]*NPCData) error {
	previous := NPCConfigInstance
	defer func() { NPCConfigInstance = previous }()
	NPCConfigInstance = &NPCConfig{NPCs: defs}
	return validateLoadedNPCConfig(NPCConfigInstance)
}
