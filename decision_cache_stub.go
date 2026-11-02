//go:build !(linux && (amd64 || arm64))

package zen

import "strings"

// InvalidateDecision only checks its input and the engine state: the libzen_ffi
// of this platform has no decision cache, every evaluation goes through the loader.
func (engine *engine) InvalidateDecision(key string) error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()

	if strings.IndexByte(key, 0) >= 0 {
		return containsNULError("decision key")
	}
	return nil
}

// ClearDecisions only checks the engine state (see InvalidateDecision).
func (engine *engine) ClearDecisions() error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()
	return nil
}
