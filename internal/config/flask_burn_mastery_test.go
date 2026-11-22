package config

import (
	"fmt"
	"testing"
)

func TestFlaskBurnMasteryValidation(t *testing.T) {
	for tier := 0; tier < 4; tier++ {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			previous := GlobalItems
			t.Cleanup(func() { GlobalItems = previous })
			_, err := LoadItemConfig("../../assets/items.yaml")
			if err != nil {
				t.Fatal(err)
			}
			d, _ := GetItemDefinition("fire_flask")
			d.Flask.BurnSeconds[tier] = -1
			if err := validateCraftedItem("fire_flask", d); err == nil {
				t.Fatal("negative mastery burn duration accepted")
			}
		})
	}
}

// Every flask, crafted or reusable, tints its burst with its brew_color.
func TestFlaskNeedsBurstTint(t *testing.T) {
	for _, key := range []string{"fire_flask", "solstice_everflow_retort"} {
		t.Run(key, func(t *testing.T) {
			previous := GlobalItems
			t.Cleanup(func() { GlobalItems = previous })
			if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
				t.Fatal(err)
			}
			d, _ := GetItemDefinition(key)
			if err := validateCraftedItem(key, d); err != nil {
				t.Fatalf("authored flask rejected: %v", err)
			}
			d.BrewColor = [3]int{}
			if err := validateCraftedItem(key, d); err == nil {
				t.Fatal("flask without brew_color accepted")
			}
		})
	}
}
