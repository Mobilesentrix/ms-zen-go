//go:build linux && (amd64 || arm64)

package zen

// #include "zen_engine.h"
import "C"
import (
	"fmt"
	"unsafe"
)

// The MobileSentrix libzen_ffi (deps/linux_amd64, deps/linux_arm64) caches the
// compiled decision of every key its loader returned and exports
// zen_engine_invalidate_decision / zen_engine_clear_decisions.

// cacheCallError maps the status of zen_engine_invalidate_decision /
// zen_engine_clear_decisions (0 ok, 1 InvalidArgument, 13 Panic).
func cacheCallError(call string, status C.uint8_t) error {
	if status == 0 {
		return nil
	}
	code := ErrorCode(status)
	return &Error{Code: code, Details: fmt.Sprintf("zen-engine: %s failed (%s)", call, code)}
}

// InvalidateDecision drops the compiled decision cached for key; the next
// evaluation of key (directly or as a sub-decision) reloads it through the
// loader. Call it whenever the loader's content for key changes. Unknown keys
// are a no-op. Safe while other goroutines evaluate.
func (engine *engine) InvalidateDecision(key string) error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()

	cKey, err := cString("decision key", key)
	if err != nil {
		return err
	}
	defer C.free(unsafe.Pointer(cKey))

	return cacheCallError("invalidate decision", C.zen_engine_invalidate_decision(engine.enginePtr, cKey))
}

// ClearDecisions drops every cached decision, e.g. after the whole rule set was
// reloaded. Safe while other goroutines evaluate.
func (engine *engine) ClearDecisions() error {
	release, err := engine.acquire()
	if err != nil {
		return err
	}
	defer release()

	return cacheCallError("clear decisions", C.zen_engine_clear_decisions(engine.enginePtr))
}
