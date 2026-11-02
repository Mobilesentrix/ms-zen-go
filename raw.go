package zen

// #include <stdlib.h>
// #include <string.h>
// #include "zen_engine.h"
import "C"
import (
	"bytes"
	"encoding/json"
	"errors"
	"unsafe"
)

// RawEvaluator evaluates with the context as JSON bytes and returns the
// engine's response JSON ({"performance", "result", "trace"?}) exactly as the
// engine wrote it, without decoding the context or the response in Go. The trace
// key is absent when options.Trace is false. The engine returned by NewEngine
// implements it.
type RawEvaluator interface {
	EvaluateRaw(key string, context []byte, options EvaluationOptions) ([]byte, error)
}

// RawDecision is RawEvaluator for a created decision.
type RawDecision interface {
	EvaluateRaw(context []byte, options EvaluationOptions) ([]byte, error)
}

// ResultEvaluator is RawEvaluator without the copy: the response JSON stays in
// the engine's memory (see RawResult). The engine returned by NewEngine
// implements it.
type ResultEvaluator interface {
	EvaluateResult(key string, context []byte, options EvaluationOptions) (*RawResult, error)
}

// ResultDecision is ResultEvaluator for a created decision.
type ResultDecision interface {
	EvaluateResult(context []byte, options EvaluationOptions) (*RawResult, error)
}

var (
	_ RawEvaluator    = (*engine)(nil)
	_ ResultEvaluator = (*engine)(nil)
	_ DecisionCache   = (*engine)(nil)
	_ RawDecision     = (*decision)(nil)
	_ ResultDecision  = (*decision)(nil)
)

// errNoResult reports an engine success without a result string, so a nil
// pointer never reaches strlen.
var errNoResult = errors.New("zen-engine: the engine returned no result")

// cView is size bytes of C memory at data as a Go slice, without copying.
func cView(data *C.char, size int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(data)), size)
}

// goBytes copies size bytes at data into Go memory. size is not limited to the
// C.int range of C.GoBytes.
func goBytes(data *C.char, size int) []byte {
	out := make([]byte, size)
	copy(out, cView(data, size))
	return out
}

// cBytes copies b into NUL-terminated C memory. Like cString it rejects a NUL
// byte. The caller frees the result with C.free.
func cBytes(what string, b []byte) (*C.char, error) {
	if bytes.IndexByte(b, 0) >= 0 {
		return nil, containsNULError(what)
	}
	buffer := (*C.char)(C.malloc(C.size_t(len(b) + 1)))
	view := cView(buffer, len(b)+1)
	copy(view, b)
	view[len(b)] = 0
	return buffer, nil
}

// takeBytes copies the engine's result string into Go memory and frees it. Its
// parameters match the results of evaluateC so that it can wrap the call.
func takeBytes(text *C.char, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(text))
	return goBytes(text, int(C.strlen(text))), nil
}

// wrapResult hands the engine's result string out as a RawResult. Its parameters
// match the results of evaluateC so that it can wrap the call.
func wrapResult(text *C.char, err error) (*RawResult, error) {
	if err != nil {
		return nil, err
	}
	return newRawResult(text), nil
}

// contextJSON is the context as JSON bytes: []byte and json.RawMessage are
// used as given, anything else is encoded.
func contextJSON(context any) ([]byte, error) {
	switch c := context.(type) {
	case []byte:
		return c, nil
	case json.RawMessage:
		return c, nil
	}
	return json.Marshal(context)
}

func (engine *engine) EvaluateRaw(key string, context []byte, options EvaluationOptions) ([]byte, error) {
	return takeBytes(engine.evaluateC(key, context, options))
}

func (engine *engine) EvaluateResult(key string, context []byte, options EvaluationOptions) (*RawResult, error) {
	return wrapResult(engine.evaluateC(key, context, options))
}

// evaluateC returns the engine's response JSON in C memory, for the caller to free.
func (engine *engine) evaluateC(key string, context []byte, options EvaluationOptions) (*C.char, error) {
	release, err := engine.acquire()
	if err != nil {
		return nil, err
	}
	defer release()

	cKey, err := cString("decision key", key)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cKey))

	cData, err := cBytes("evaluation context", context)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cData))

	resultPtr := C.zen_engine_evaluate(engine.enginePtr, cKey, cData, cEvaluationOptions(options))
	if resultPtr.error > 0 {
		return nil, resultError(resultPtr.error, resultPtr.details)
	}
	if resultPtr.result == nil {
		return nil, errNoResult
	}
	return resultPtr.result, nil
}

func (decision *decision) EvaluateRaw(context []byte, options EvaluationOptions) ([]byte, error) {
	return takeBytes(decision.evaluateC(context, options))
}

func (decision *decision) EvaluateResult(context []byte, options EvaluationOptions) (*RawResult, error) {
	return wrapResult(decision.evaluateC(context, options))
}

// evaluateC returns the decision's response JSON in C memory, for the caller to free.
func (decision *decision) evaluateC(context []byte, options EvaluationOptions) (*C.char, error) {
	decision.mu.RLock()
	defer decision.mu.RUnlock()
	if decision.disposed {
		return nil, ErrDecisionDisposed
	}
	return decision.evaluateCLocked(context, options)
}

// evaluateCLocked is evaluateC for a caller that holds decision.mu for reading.
func (decision *decision) evaluateCLocked(context []byte, options EvaluationOptions) (*C.char, error) {
	cData, err := cBytes("evaluation context", context)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cData))

	resultPtr := C.zen_decision_evaluate(decision.decisionPtr, cData, cEvaluationOptions(options))
	if resultPtr.error > 0 {
		return nil, resultError(resultPtr.error, resultPtr.details)
	}
	if resultPtr.result == nil {
		return nil, errNoResult
	}
	return resultPtr.result, nil
}

func decodeResponse(raw []byte) (*EvaluationResponse, error) {
	var response EvaluationResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
