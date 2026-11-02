package zen

// #include <stdlib.h>
// #include <string.h>
// #include "zen_engine.h"
import "C"
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"unsafe"
)

// RawEvaluator evaluates with the context as JSON bytes and returns the
// engine's response JSON ({"performance", "result", "trace"?}) exactly as the
// engine wrote it. It skips the Go map and both Go JSON round trips of
// EvaluateWithOpts; the trace key is absent when options.Trace is false.
// The engine returned by NewEngine implements it.
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

// RawResult is an engine response JSON that is not copied into Go memory: a
// traced response can be tens of megabytes, and copying it (then copying it
// again into an HTTP response) would grow the Go heap by several times that per
// request. It reads like a bytes.Reader over the engine's buffer, and Close
// frees the buffer; after Close it reads as empty. Close it exactly when the
// bytes are no longer needed (an HTTP server closes a body stream once it has
// written it). A RawResult that is never closed is freed when it is garbage
// collected.
type RawResult struct {
	mu   sync.Mutex
	p    *C.char
	n    int
	read int
}

func newRawResult(p *C.char) *RawResult {
	r := &RawResult{p: p, n: int(C.strlen(p))} // p is non-nil (see evaluateC)
	runtime.SetFinalizer(r, (*RawResult).Close)
	return r
}

// Len is the size of the JSON in bytes.
func (r *RawResult) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// Read implements io.Reader over the JSON.
func (r *RawResult) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.p == nil || r.read >= r.n {
		return 0, io.EOF
	}
	view := unsafe.Slice((*byte)(unsafe.Pointer(r.p)), r.n)
	k := copy(b, view[r.read:])
	r.read += k
	return k, nil
}

// LastByte is the final byte of the JSON (false when it is empty or closed).
func (r *RawResult) LastByte() (byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.p == nil || r.n == 0 {
		return 0, false
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(r.p)), r.n)[r.n-1], true
}

// Bytes copies the JSON into Go memory.
func (r *RawResult) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.p == nil {
		return nil
	}
	return goBytes(r.p, r.n)
}

// goBytes copies n bytes at p into Go memory. Unlike C.GoBytes it takes an int,
// so a response over 2 GiB is copied whole instead of overflowing C.int.
func goBytes(p *C.char, n int) []byte {
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
	return out
}

// Close frees the engine's buffer; later calls do nothing.
func (r *RawResult) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.p != nil {
		C.free(unsafe.Pointer(r.p))
		r.p, r.n, r.read = nil, 0, 0
		runtime.SetFinalizer(r, nil)
	}
	return nil
}

// cBytes copies b into NUL-terminated C memory in one copy. Like cString it
// rejects a NUL byte, which the engine would read as the end of the string.
// The caller frees the result with C.free.
func cBytes(what string, b []byte) (*C.char, error) {
	if bytes.IndexByte(b, 0) >= 0 {
		return nil, fmt.Errorf("zen-engine: %s contains a NUL byte", what)
	}
	p := (*C.char)(C.malloc(C.size_t(len(b) + 1)))
	buf := unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)+1)
	copy(buf, b)
	buf[len(b)] = 0
	return p, nil
}

// it. The copying APIs use it directly: a RawResult finalizer costs about as
// much as a small evaluation.
func takeBytes(p *C.char, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(p))
	return goBytes(p, int(C.strlen(p))), nil
}

// errNoResult: the engine reported success without a result string (never
// expected; guarded so a nil pointer never reaches strlen).
var errNoResult = errors.New("zen-engine: the engine returned no result")

// wrapResult hands the engine's result out as a RawResult.
func wrapResult(p *C.char, err error) (*RawResult, error) {
	if err != nil {
		return nil, err
	}
	return newRawResult(p), nil
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

// evaluateC is the engine's result JSON in C memory, for the caller to free.
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

func (decision *decision) evaluateC(context []byte, options EvaluationOptions) (*C.char, error) {
	decision.mu.RLock()
	defer decision.mu.RUnlock()
	if decision.disposed {
		return nil, ErrDecisionDisposed
	}
	return decision.evaluateRawLocked(context, options)
}

// evaluateRawLocked runs with decision.mu held for reading.
func (decision *decision) evaluateRawLocked(context []byte, options EvaluationOptions) (*C.char, error) {
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
