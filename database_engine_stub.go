//go:build !(linux && (amd64 || arm64))

package zen

import "log"

// DatabaseSupported reports whether the libzen_ffi linked for this platform
// contains the MobileSentrix database plug-in. Only deps/linux_amd64 and
// deps/linux_arm64 carry it; other platforms use upstream gorules builds.
const DatabaseSupported = false

// NewEngineWithDatabases creates an engine with the database plug-in. On this
// platform the plug-in is not linked: the engine is created without it and every
// database node fails at evaluation with ErrDatabaseNodeUnavailable; all other
// node types work normally.
func NewEngineWithDatabases(config EngineConfig, database DatabaseConfig) (Engine, error) {
	if err := database.validate(); err != nil {
		return nil, err
	}

	log.Printf("zen-engine: %v on this platform; database nodes will return node errors", ErrDatabaseNodeUnavailable)

	config.CustomNodeHandler = DatabaseNodeUnavailableHandler(config.CustomNodeHandler)
	return NewEngine(config), nil
}
