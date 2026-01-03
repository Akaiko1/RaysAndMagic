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
		BonusMight:                def.BonusMight,
		BonusIntellect:            def.BonusIntellect,
		BonusPersonality:          def.BonusPersonality,
		BonusEndurance:            def.BonusEndurance,
		BonusAccuracy:             def.BonusAccuracy,
		BonusSpeed:                def.BonusSpeed,
		BonusLuck:                 def.BonusLuck,
		HealBase:                  def.HealBase,
		HealEnduranceDivisor:      def.HealEnduranceDivisor,
		SummonDistanceTiles:       def.SummonDistanceTiles,
		EquipSlot:                 def.EquipSlot,
		Value:                     def.Value,
		Revive:                    def.Revive,
		FullHeal:                  def.FullHeal,
		OpensMap:                  def.OpensMap,
	}
	return adapted, true
}
