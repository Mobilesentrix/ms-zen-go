# Known limitations

Found by reading the code, not fixed.

- `database_engine_stub.go` `NewEngineWithDatabases`: on platforms without the plug-in it returns `NewEngine(config), nil`, so a loader or init failure appears at first evaluation. The Linux build returns it from the constructor (possible bug).
- `database.go` `DatabaseConfig.validate`: iterates a map, so when several connections have no URL, the name in the error varies between runs (possible bug).
- `engine.go` `acquire`: returns the method value `engine.mu.RUnlock`, likely one small allocation per evaluation (not measured).
- `raw.go` `fillCBatchRequest`: calls `fmt.Sprintf` twice per request for error labels used only on failure.
