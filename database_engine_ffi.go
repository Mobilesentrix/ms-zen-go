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
// the MobileSentrix database plug-in (MS-FFI.md of the zen fork). Pools are created
// once per engine; create one engine per process and reuse it. Custom nodes of
// any other kind go to config.CustomNodeHandler.
func NewEngineWithDatabases(config EngineConfig, database DatabaseConfig) (Engine, error) {
	databaseJSON, err := database.toJSON()
	if err != nil {
		return nil, err
	}

	// The JSON (connection URLs included) is only read during the constructor
	// call (MS-FFI.md §1) and is freed right after; it is never logged.
	cDatabase, err := cString("database config", databaseJSON)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(cDatabase))

	newEngine := &engine{}
	var loaderHandlerIdPtr C.uintptr_t
	var customNodeHandlerIdPtr C.uintptr_t

	if config.CustomNodeHandler != nil {
		customNodeHandlerIdPtr = newEngine.installCustomNodeHandler(config.CustomNodeHandler)
	}

	var result C.ZenResult_ZenEngineStruct
	switch loader := config.Loader.(type) {
	case nil:
		result = C.zen_engine_new_golang_with_database(&loaderHandlerIdPtr, &customNodeHandlerIdPtr, cDatabase)
	case Loader:
		loaderHandlerIdPtr = newEngine.installLoader(loader)
		result = C.zen_engine_new_golang_with_database(&loaderHandlerIdPtr, &customNodeHandlerIdPtr, cDatabase)
	case LoaderConfig:
		cConfig, freeConfig, err := loader.cLoaderConfig()
		if err != nil {
			newEngine.disposeHandlers()
			return nil, err
		}
		defer freeConfig()
		result = C.zen_engine_new_golang_with_loader_config_and_database(cConfig, &customNodeHandlerIdPtr, cDatabase)
	default:
		newEngine.disposeHandlers()
		return nil, fmt.Errorf("unsupported loader type %T", config.Loader)
	}

	if result.error > 0 {
		newEngine.disposeHandlers()
		return nil, fmt.Errorf("zen-engine: database engine creation failed (code %d): %w", result.error, resultError(result.error, result.details))
	}

	newEngine.enginePtr = result.result
	return newEngine, nil
}
