package zen

// #include "zen_engine.h"
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/cgo"
	"strings"
	"sync"
	"unsafe"
)

// ErrEngineDisposed is returned by every Engine method called after Dispose.
var ErrEngineDisposed = errors.New("zen-engine: engine has been disposed")

// engine wraps the Rust ZenEngine. All methods are safe for concurrent use:
// evaluations hold mu for reading while the Rust engine pointer is in use, and
// Dispose takes it for writing, so the pointer is never freed under an
// in-flight call and a second Dispose is a no-op.
type engine struct {
	mu       sync.RWMutex
	disposed bool

	loaderHandler        cgo.Handle
	hasLoaderHandler     bool
	customNodeHandler    cgo.Handle
	hasCustomNodeHandler bool
	enginePtr            *C.ZenEngineStruct
	initError            error
}

type EngineConfig struct {
	Loader            EngineLoader
	CustomNodeHandler CustomNodeHandler
}

type EngineLoader interface {
	engineLoader()
}

type LoaderConfig interface {
	EngineLoader
	cLoaderConfig() (C.ZenEngineLoaderConfig, func(), error)
}

type StaticLoader struct {
	Content map[string]json.RawMessage
}

func (StaticLoader) engineLoader() {}

func (config StaticLoader) cLoaderConfig() (C.ZenEngineLoaderConfig, func(), error) {
	data, err := json.Marshal(config.Content)
	if err != nil {
		return C.ZenEngineLoaderConfig{}, nil, err
	}

	content, err := cString("static loader content", string(data))
	if err != nil {
		return C.ZenEngineLoaderConfig{}, nil, err
	}
	return C.ZenEngineLoaderConfig{
		kind:    C.ZenLoaderConfigKind_Static,
		content: content,
	}, func() { C.free(unsafe.Pointer(content)) }, nil
}

type FilesystemLoader struct {
	Path string
}

func (FilesystemLoader) engineLoader() {}

func (config FilesystemLoader) cLoaderConfig() (C.ZenEngineLoaderConfig, func(), error) {
	content, err := cString("filesystem loader path", config.Path)
	if err != nil {
		return C.ZenEngineLoaderConfig{}, nil, err
	}
	return C.ZenEngineLoaderConfig{
		kind:    C.ZenLoaderConfigKind_Filesystem,
		content: content,
	}, func() { C.free(unsafe.Pointer(content)) }, nil
}

type ZipLoader struct {
	Bytes []byte
}

func (ZipLoader) engineLoader() {}

func (config ZipLoader) cLoaderConfig() (C.ZenEngineLoaderConfig, func(), error) {
	if len(config.Bytes) == 0 {
		return C.ZenEngineLoaderConfig{}, nil, errors.New("zip loader config requires bytes")
	}

	bytes := C.CBytes(config.Bytes)
	return C.ZenEngineLoaderConfig{
		kind:      C.ZenLoaderConfigKind_Zip,
		bytes:     (*C.uint8_t)(bytes),
		bytes_len: C.uintptr_t(len(config.Bytes)),
	}, func() { C.free(bytes) }, nil
}

// cString copies s into C memory. C strings end at the first NUL byte, so a
// value containing one would be silently truncated on the Rust side; it is
// rejected instead. The caller frees the result with C.free.
func cString(what string, s string) (*C.char, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return nil, fmt.Errorf("zen-engine: %s contains a NUL byte", what)
	}
	return C.CString(s), nil
}

// The two exports below are called from Rust (inside the engine's tokio
// runtime). A Go panic must not unwind through those foreign frames: it would
// skip Rust destructors and leave the runtime in an undefined state. recover
// converts it into an error result the engine reports as a loader/node error.

//export zen_engine_go_loader_callback
func zen_engine_go_loader_callback(h C.uintptr_t, key *C.char) (result C.ZenDecisionLoaderResult) {
	defer func() {
		if r := recover(); r != nil {
			result = C.ZenDecisionLoaderResult{
				content: nil,
				error:   C.CString(fmt.Sprintf("panic in loader: %v", r)),
			}
		}
	}()

	fn := cgo.Handle(h).Value().(func(*C.char) C.ZenDecisionLoaderResult)
	return fn(key)
}

//export zen_engine_go_custom_node_callback
func zen_engine_go_custom_node_callback(h C.uintptr_t, request *C.char) (result C.ZenCustomNodeResult) {
	defer func() {
		if r := recover(); r != nil {
			result = C.ZenCustomNodeResult{
				content: nil,
				error:   C.CString(fmt.Sprintf("panic in custom node handler: %v", r)),
			}
		}
	}()

	fn := cgo.Handle(h).Value().(func(*C.char) C.ZenCustomNodeResult)
	return fn(request)
}

func NewEngine(config EngineConfig) Engine {
	newEngine := &engine{}
	var loaderHandlerIdPtr C.uintptr_t
	var customNodeHandlerIdPtr C.uintptr_t

	var loaderCallback Loader
	var loaderConfig LoaderConfig

	switch loader := config.Loader.(type) {
	case nil:
	case Loader:
		loaderCallback = loader
	case LoaderConfig:
		loaderConfig = loader
	default:
		return &engine{initError: fmt.Errorf("unsupported loader type %T", config.Loader)}
	}

	if config.CustomNodeHandler != nil {
		customNodeHandlerIdPtr = newEngine.installCustomNodeHandler(config.CustomNodeHandler)
	}

	if loaderConfig != nil {
		cConfig, freeConfig, err := loaderConfig.cLoaderConfig()
		if err != nil {
			newEngine.disposeHandlers()
			return &engine{initError: err}
		}
		defer freeConfig()

		// The handle ids are read by Rust during the call only (Option<&usize>
		// is cloned immediately); no Go pointer is retained.
		resultPtr := C.zen_engine_new_golang_with_loader_config(cConfig, &customNodeHandlerIdPtr)
		if resultPtr.error > 0 {
			newEngine.disposeHandlers()
			return &engine{initError: resultError(resultPtr.error, resultPtr.details)}
		}

		newEngine.enginePtr = resultPtr.result
		return newEngine
	}

	if loaderCallback != nil {
		loaderHandlerIdPtr = newEngine.installLoader(loaderCallback)
	}

	newEngine.enginePtr = C.zen_engine_new_golang(&loaderHandlerIdPtr, &customNodeHandlerIdPtr)
	if newEngine.enginePtr == nil {
		// The raw-pointer constructor returns NULL when a Rust panic was caught
		// at the C boundary (error code 13).
		newEngine.disposeHandlers()
		return &engine{initError: &Error{Code: ErrorCodePanic, Details: "zen-engine: engine creation failed (panic in libzen_ffi)"}}
	}
	return newEngine
}

func (engine *engine) installLoader(loader Loader) C.uintptr_t {
	engine.loaderHandler = cgo.NewHandle(wrapLoader(loader))
	engine.hasLoaderHandler = true
	return C.uintptr_t(engine.loaderHandler)
}

func (engine *engine) installCustomNodeHandler(handler CustomNodeHandler) C.uintptr_t {
	engine.customNodeHandler = cgo.NewHandle(wrapCustomNodeHandler(handler))
	engine.hasCustomNodeHandler = true
	return C.uintptr_t(engine.customNodeHandler)
}

// resultError turns a ZenResult error into an *Error, freeing details.
func resultError(code C.uint8_t, details *C.char) error {
	if details == nil {
		return &Error{Code: ErrorCode(code), Details: fmt.Sprintf("Error code: %d", code)}
	}
	defer C.free(unsafe.Pointer(details))
	return &Error{Code: ErrorCode(code), Details: C.GoString(details)}
}

// disposeHandlers deletes the cgo handles exactly once. Callers hold mu or own
// the engine exclusively (construction).
func (engine *engine) disposeHandlers() {
	if engine.hasLoaderHandler {
		engine.loaderHandler.Delete()
		engine.hasLoaderHandler = false
	}

	if engine.hasCustomNodeHandler {
		engine.customNodeHandler.Delete()
		engine.hasCustomNodeHandler = false
	}
}

// acquire pins the engine for one FFI call. The returned release must be
// called when the call has returned.
func (engine *engine) acquire() (release func(), err error) {
	if engine.initError != nil {
		return nil, engine.initError
	}

	engine.mu.RLock()
	if engine.disposed {
		engine.mu.RUnlock()
		return nil, ErrEngineDisposed
	}
	return engine.mu.RUnlock, nil
}

func (engine *engine) Evaluate(key string, context any) (*EvaluationResponse, error) {
	return engine.EvaluateWithOpts(key, context, EvaluationOptions{})
}

func (engine *engine) EvaluateWithOpts(key string, context any, options EvaluationOptions) (*EvaluationResponse, error) {
	if engine.initError != nil {
		return nil, engine.initError
	}
	jsonData, err := contextJSON(context)
	if err != nil {
		return nil, err
	}
	raw, err := engine.EvaluateRaw(key, jsonData, options)
	if err != nil {
		return nil, err
	}
	return decodeResponse(raw)
}

func (engine *engine) EvaluateBatch(requests []EvaluateBatchRequest) ([]EvaluateBatchResult, error) {
	return engine.EvaluateBatchWithOpts(requests, EvaluationOptions{})
}

func (engine *engine) EvaluateBatchWithOpts(requests []EvaluateBatchRequest, options EvaluationOptions) ([]EvaluateBatchResult, error) {
	release, err := engine.acquire()
	if err != nil {
		return nil, err
	}
	defer release()

	if len(requests) == 0 {
		return []EvaluateBatchResult{}, nil
	}

	cRequests := make([]C.ZenEngineEvaluateBatchRequest, len(requests))
	defer func() {
		for _, request := range cRequests {
			if request.key != nil {
				C.free(unsafe.Pointer(request.key))
			}
			if request.context != nil {
				C.free(unsafe.Pointer(request.context))
			}
		}
	}()

	for i, request := range requests {
		jsonData, err := extractJsonFromAny(request.Context)
		if err != nil {
			return nil, err
		}

		cKey, err := cString(fmt.Sprintf("decision key of request %d", i), request.Key)
		if err != nil {
			return nil, err
		}
		cRequests[i].key = cKey

		cContext, err := cString(fmt.Sprintf("evaluation context of request %d", i), string(jsonData))
		if err != nil {
			return nil, err
		}
		cRequests[i].context = cContext
	}

	resultPtr := C.zen_engine_evaluate_batch(engine.enginePtr, &cRequests[0], C.uintptr_t(len(requests)), cEvaluationOptions(options))
	if resultPtr.error > 0 {
		return nil, resultError(resultPtr.error, resultPtr.details)
	}

	defer C.free(unsafe.Pointer(resultPtr.result))
	result := C.GoString(resultPtr.result)

	var response []EvaluateBatchResult
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		return nil, err
	}

	return response, nil
}

func (engine *engine) GetDecision(key string) (Decision, error) {
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

	decisionPtr := C.zen_engine_get_decision(engine.enginePtr, cKey)
	if decisionPtr.error > 0 {
		return nil, resultError(decisionPtr.error, decisionPtr.details)
	}

	return newDecision(decisionPtr.result), nil
}

func (engine *engine) CreateDecision(data []byte) (Decision, error) {
	release, err := engine.acquire()
	if err != nil {
		return nil, err
	}
	defer release()

	cData, err := cString("decision content", string(data))
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cData))

	decisionPtr := C.zen_engine_create_decision(engine.enginePtr, cData)
	if decisionPtr.error > 0 {
		return nil, resultError(decisionPtr.error, decisionPtr.details)
	}

	return newDecision(decisionPtr.result), nil
}

// Dispose frees the Rust engine and the callback handles. It waits for
// in-flight calls to return and is safe to call more than once. A Loader or
// CustomNodeHandler runs inside an engine call, so it must not call Dispose on
// the same engine (that call would wait for itself).
func (engine *engine) Dispose() {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	if engine.disposed {
		return
	}
	engine.disposed = true

	if engine.enginePtr != nil {
		C.zen_engine_free(engine.enginePtr)
		engine.enginePtr = nil
	}

	engine.disposeHandlers()
}

func cEvaluationOptions(options EvaluationOptions) C.ZenEngineEvaluationOptions {
	maxDepth := options.MaxDepth
	if maxDepth == 0 {
		maxDepth = 1
	}

	return C.ZenEngineEvaluationOptions{
		trace:     C.bool(options.Trace),
		max_depth: C.uint8_t(maxDepth),
	}
}
