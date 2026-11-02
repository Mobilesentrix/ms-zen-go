//go:build linux && (amd64 || arm64)

package zen

// #include "zen_engine.h"
import "C"
import (
	"fmt"
	"unsafe"
)

// DatabaseSupported reports whether the libzen_ffi linked for this platform
// contains the MobileSentrix database plug-in (deps/linux_amd64 and
// deps/linux_arm64 do).
const DatabaseSupported = true

// NewEngineWithDatabases creates an engine whose database nodes run natively in
// the MobileSentrix database plug-in. Pools are created once per engine; create
// one engine per process and reuse it. Custom nodes of any other kind go to
// config.CustomNodeHandler.
func NewEngineWithDatabases(config EngineConfig, database DatabaseConfig) (Engine, error) {
	databaseJSON, err := database.toJSON()
	if err != nil {
		return nil, err
	}

	// Holds connection URLs: read only during the constructor call, never logged.
	cDatabase, err := cString("database config", databaseJSON)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cDatabase))

	newEngine := &engine{}
	var customNodeHandlerID C.uintptr_t
	if config.CustomNodeHandler != nil {
		customNodeHandlerID = newEngine.installCustomNodeHandler(config.CustomNodeHandler)
	}

	result, err := newEngine.createWithDatabase(config.Loader, customNodeHandlerID, cDatabase)
	if err != nil {
		newEngine.disposeHandlers()
		return nil, err
	}
	if result.error > 0 {
		newEngine.disposeHandlers()
		return nil, fmt.Errorf("zen-engine: database engine creation failed (code %d): %w", result.error, resultError(result.error, result.details))
	}

	newEngine.enginePtr = result.result
	return newEngine, nil
}

// createWithDatabase calls the Rust constructor that fits loader, installing a
// callback loader on the engine first.
func (engine *engine) createWithDatabase(loader EngineLoader, customNodeHandlerID C.uintptr_t, cDatabase *C.char) (C.ZenResult_ZenEngineStruct, error) {
	var loaderHandlerID C.uintptr_t

	switch loader := loader.(type) {
	case nil:
	case Loader:
		loaderHandlerID = engine.installLoader(loader)
	case LoaderConfig:
		cConfig, freeConfig, err := loader.cLoaderConfig()
		if err != nil {
			return C.ZenResult_ZenEngineStruct{}, err
		}
		defer freeConfig()
		return C.zen_engine_new_golang_with_loader_config_and_database(cConfig, &customNodeHandlerID, cDatabase), nil
	default:
		return C.ZenResult_ZenEngineStruct{}, fmt.Errorf("unsupported loader type %T", loader)
	}

	return C.zen_engine_new_golang_with_database(&loaderHandlerID, &customNodeHandlerID, cDatabase), nil
}
