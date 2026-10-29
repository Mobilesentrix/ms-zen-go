//go:build !(linux && (amd64 || arm64))

package zen

import (
	"errors"
	"strings"
)

// The upstream gorules libzen_ffi used on this platform has no compiled-decision
// cache for loader engines (every evaluation goes through the loader) and does
// not export the invalidate/clear functions, so there is nothing to drop: both
// methods only check their input and the engine state, keeping the linux contract.

// InvalidateDecision is a no-op on this platform (no decision cache).
func (engine *engine) InvalidateDecision(key string) error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()

	if strings.IndexByte(key, 0) >= 0 {
		return errors.New("zen-engine: decision key contains a NUL byte")
	}
	return nil
}

// ClearDecisions is a no-op on this platform (no decision cache).
func (engine *engine) ClearDecisions() error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()
	return nil
}
