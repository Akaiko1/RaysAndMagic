package bridge

import (
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// SetupItemBridge configures the item accessor bridge
func SetupItemBridge() {
	items.GlobalItemAccessor = getItemFromConfig
}

// getItemFromConfig retrieves item definition from config and adapts it
func getItemFromConfig(itemKey string) (*items.ItemDefinitionFromYAML, bool) {
	def, exists := config.GetItemDefinition(itemKey)
	if !exists || def == nil {
		return nil, false
	}
	adapted := &items.ItemDefinitionFromYAML{
		UseAction:                 def.UseAction,
		UseSpell:                  def.UseSpell,
		UseJumpExtraTiles:         def.UseJumpExtraTiles,
		UseCooldownSeconds:        def.UseCooldownSeconds,
		Name:                      def.Name,
		Description:               def.Description,
		Flavor:                    def.Flavor,
		Type:                      def.Type,
		ArmorType:                 def.ArmorType,
		Rarity:                    def.Rarity,
		ArmorClassBase:            def.ArmorClassBase,
		EnduranceScalingDivisor:   def.EnduranceScalingDivisor,
		IntellectScalingDivisor:   def.IntellectScalingDivisor,
		PersonalityScalingDivisor: def.PersonalityScalingDivisor,
		StatBonus:                 def.StatBonus.Stats(),
		Resistances:               def.Resistances,
		HealBase:                  def.HealBase,
		HealEnduranceDivisor:      def.HealEnduranceDivisor,
		SummonDistanceTiles:       def.SummonDistanceTiles,
		EquipSlot:                 def.EquipSlot,
		Value:                     def.Value,
		Revive:                    def.Revive,
		FullHeal:                  def.FullHeal,
		CurePoison:                def.CurePoison,
		DoorKey:                   def.DoorKey,
		MasterKey:                 def.MasterKey,
		OpensMap:                  def.OpensMap,
		PromotesLich:              def.PromotesLich,
		Discardable:               def.Discardable,
		Set:                       def.Set,
		PartyArmorBonus:           def.PartyArmorBonus,
		ManaBase:                  def.ManaBase,
		ManaPersonalityDivisor:    def.ManaPersonalityDivisor,
	}
	return adapted, true
}
