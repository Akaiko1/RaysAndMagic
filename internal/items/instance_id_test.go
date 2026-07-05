package items

import "testing"

func TestEnsureInstanceID(t *testing.T) {
	var it Item // empty (no name)
	if EnsureInstanceID(&it) || it.InstanceID != 0 {
		t.Errorf("empty item should not be stamped, got id=%d", it.InstanceID)
	}
	if EnsureInstanceID(nil) {
		t.Error("nil item should return false")
	}

	named := Item{Name: "Sword"}
	if !EnsureInstanceID(&named) || named.InstanceID == 0 {
		t.Fatalf("named item should be stamped, got id=%d", named.InstanceID)
	}
	first := named.InstanceID
	if EnsureInstanceID(&named) || named.InstanceID != first {
		t.Errorf("already-identified item must be untouched, got id=%d", named.InstanceID)
	}
}

func TestNewInstanceID_UniqueNonZero(t *testing.T) {
	seen := make(map[uint64]bool)
	for i := 0; i < 10000; i++ {
		id := NewInstanceID()
		if id == 0 {
			t.Fatal("NewInstanceID returned the 0 sentinel")
		}
		if seen[id] {
			t.Fatalf("duplicate instance id %d at iteration %d", id, i)
		}
		seen[id] = true
	}
}

func TestFactoryAssignsInstanceID(t *testing.T) {
	old := GlobalWeaponAccessor
	defer func() { GlobalWeaponAccessor = old }()
	GlobalWeaponAccessor = func(key string) (*WeaponDefinitionFromYAML, bool) {
		return &WeaponDefinitionFromYAML{Name: "Test Blade", Category: "sword", Rarity: "common"}, true
	}

	a := CreateWeaponFromYAML("test")
	b := CreateWeaponFromYAML("test")
	if a.InstanceID == 0 || b.InstanceID == 0 {
		t.Fatalf("factory must stamp ids, got a=%d b=%d", a.InstanceID, b.InstanceID)
	}
	if a.InstanceID == b.InstanceID {
		t.Errorf("two crafted items share an id %d — not unique instances", a.InstanceID)
	}
}

func TestInstanceIDGenSwappable(t *testing.T) {
	old := instanceIDGen
	defer func() { instanceIDGen = old }()
	var n uint64
	instanceIDGen = func() uint64 { n++; return n }

	if got := NewInstanceID(); got != 1 {
		t.Errorf("swapped gen: got %d, want 1", got)
	}
	it := Item{Name: "x"}
	EnsureInstanceID(&it)
	if it.InstanceID != 2 {
		t.Errorf("swapped gen via EnsureInstanceID: got %d, want 2", it.InstanceID)
	}
}
