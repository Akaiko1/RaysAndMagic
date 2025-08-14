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
        Name:        def.Name,
        Description: def.Description,
        Type:        def.Type,
        ArmorClassBase:            def.ArmorClassBase,
        EnduranceScalingDivisor:   def.EnduranceScalingDivisor,
        IntellectScalingDivisor:   def.IntellectScalingDivisor,
        PersonalityScalingDivisor: def.PersonalityScalingDivisor,
    }
    return adapted, true
}
