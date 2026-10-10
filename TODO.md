# TODO

- [ ] Review the 44 production functions that only tests call (staticcheck
  U1000 and deadcode do not flag them; list verified 2026-10-10 against all
  .go files, untracked included). For each, decide one of three outcomes:
  keep it as a deliberate test seam; move it into a _test.go file or point
  the test at the production path (string wrappers over the `*Rows` card
  builders); or, if the game should call it, wire the call (a helper-only
  test cannot see a missing call site).
  - character: IsArchmage, WeaponDamageBreakdown, ConsumeOneAt,
    GetTotalItems, RaceOwned
  - config: AlchemyRecipeByKey, GetBossLoot
  - game tooltips: GetItemTooltip, GetItemComparisonTooltip, GetSpellTooltip,
    GetSpellComparisonTooltip, buildSpellComparisonLinesByID,
    buildWeaponTooltipUnified, buildSpellTooltipUnified,
    buildTrapTooltipUnified, buildSimpleItemTooltipWithParty,
    cardItemTooltipLines, spellTraderTooltipLines, buffStatusCard,
    statTooltipText, masteryTooltipTextForSkill, magicMasteryTooltipText,
    queueTooltipComparison, queueTitledTooltipComparison
  - game other: createArrowAttack, GetCombatMessages, GetCombatMessageColor,
    enterEncounterMap, CalculateMonsterSpriteMetrics,
    collectMapRenderPrewarmPlan, estimatedGPUBytes, floorTextureIndexForTile,
    inventoryBagUnits, keytracker.NewWithSource
  - other packages: graphics PreparationBudget.Usage, monster TakeDamageParts,
    playerprofile ResetAchievements, quests OnMonsterKilled,
    sound SetMusicBiome, storage SetDataRootForTesting,
    testutil UniformVisualSizeClasses,
    threading NewEntityUpdaterWithWorkers, world OpeningInteriorCells,
    world GetAllTileKeys
