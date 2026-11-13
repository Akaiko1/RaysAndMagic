package gpupref

import "testing"

func TestChoiceKeepsThePlayersEntry(t *testing.T) {
	if value, store := choice(false); !store || value != "GpuPreference=2;" {
		t.Fatalf("no entry: store=%v value=%q, want the high-performance value", store, value)
	}
	if value, store := choice(true); store || value != "" {
		t.Fatalf("existing entry: store=%v value=%q, want it left alone", store, value)
	}
}
