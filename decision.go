package zen

// #include "zen_engine.h"
import "C"
import (
	"errors"
	"sync"
)

// ErrDecisionDisposed is returned by Decision.Evaluate after Dispose.
var ErrDecisionDisposed = errors.New("zen-engine: decision has been disposed")

// decision wraps a Rust ZenDecision. Evaluations hold mu for reading while the
// pointer is in use; Dispose takes it for writing, waits for them, frees the
// decision once and fences later calls.
type decision struct {
	mu          sync.RWMutex
	disposed    bool
	decisionPtr *C.ZenDecisionStruct
}

// newDecision: called internally by zen_engine only, cleanup should still be fired however.
func newDecision(decisionPtr *C.ZenDecisionStruct) Decision {
	return &decision{
		decisionPtr: decisionPtr,
	}
}

func (decision *decision) Evaluate(context any) (*EvaluationResponse, error) {
	return decision.EvaluateWithOpts(context, EvaluationOptions{})
}

func (decision *decision) EvaluateWithOpts(context any, options EvaluationOptions) (*EvaluationResponse, error) {
	decision.mu.RLock()
	defer decision.mu.RUnlock()
	if decision.disposed {
		return nil, ErrDecisionDisposed
	}

	jsonData, err := contextJSON(context)
	if err != nil {
		return nil, err
	}
	raw, err := takeBytes(decision.evaluateRawLocked(jsonData, options))
	if err != nil {
		return nil, err
	}
	return decodeResponse(raw)
}

// Dispose frees the Rust decision. It waits for in-flight evaluations and is
// safe to call more than once.
func (decision *decision) Dispose() {
	decision.mu.Lock()
	defer decision.mu.Unlock()

	if decision.disposed {
		return
	}
	decision.disposed = true

	if decision.decisionPtr != nil {
		C.zen_decision_free(decision.decisionPtr)
		decision.decisionPtr = nil
	}
}
