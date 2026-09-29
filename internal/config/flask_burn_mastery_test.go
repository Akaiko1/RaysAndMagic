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
