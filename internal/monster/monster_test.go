package monster

import (
	"testing"
)


func TestNewMonster3DFromConfig_Valid(t *testing.T) {
	   // This assumes TestMain loads the config and 'goblin' exists in monsters.yaml
	   defer func() {
			   if r := recover(); r != nil {
					   t.Errorf("Did not expect panic with valid config: %v", r)
			   }
	   }()
	   m := NewMonster3DFromConfig(1, 2, "goblin", nil)
	   if m == nil || m.Name == "" {
			   t.Error("Expected valid Monster3D instance with name")
	   }
}
