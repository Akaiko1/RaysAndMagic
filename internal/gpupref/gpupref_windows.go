//go:build windows

package gpupref

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const preferencesKey = `Software\Microsoft\DirectX\UserGpuPreferences`

// PreferHighPerformance stores the high-performance preference for this
// executable unless Graphics settings already hold an entry for it. It reports
// whether it stored one; Windows applies the entry to processes started later.
func PreferHighPerformance() (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, preferencesKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer key.Close()
	_, _, err = key.GetStringValue(executable)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return false, err
	}
	value, store := choice(err == nil)
	if !store {
		return false, nil
	}
	return true, key.SetStringValue(executable, value)
}
