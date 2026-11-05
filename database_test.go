package zen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const databaseNodeGraph = `{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request", "position": {"x": 0, "y": 0}},
    {"id": "db", "type": "customNode", "name": "Orders", "position": {"x": 0, "y": 0},
     "content": {"kind": "database", "config": {"query": "SELECT 1 AS one", "outputPath": "dbResult", "passThrough": true}}},
    {"id": "out", "type": "outputNode", "name": "Response", "position": {"x": 0, "y": 0}}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "db", "type": "edge"},
    {"id": "e2", "sourceId": "db", "targetId": "out", "type": "edge"}
  ]
}`

func evaluateGraph(t *testing.T, engine Engine) error {
	t.Helper()
	decision, err := engine.CreateDecision([]byte(databaseNodeGraph))
	require.NoError(t, err)
	defer decision.Dispose()

	_, err = decision.Evaluate(map[string]any{"id": 1})
	return err
}

func TestDatabaseConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		wantErr string
	}{
		{"nil connections", DatabaseConfig{}, "requires connections"},
		{"missing url", DatabaseConfig{Connections: map[string]DatabaseConnection{"a": {}}}, "has no URL"},
		{"unknown default", DatabaseConfig{Connections: map[string]DatabaseConnection{}, Default: "x"}, "not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEngineWithDatabases(EngineConfig{}, tt.config)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}

	_, err := NewEngineWithDatabase(EngineConfig{}, "", DatabaseOptions{})
	assert.ErrorContains(t, err, "database URL is required")
}

func TestEngineWithoutDatabase_DatabaseNodeUnavailable(t *testing.T) {
	engine := NewEngine(EngineConfig{CustomNodeHandler: DatabaseNodeUnavailableHandler(nil)})
	defer engine.Dispose()

	assert.ErrorContains(t, evaluateGraph(t, engine), "database node not available")
}

func TestNewEngineWithDatabase_LazyPool(t *testing.T) {
	// Pools connect lazily: creation succeeds with an unreachable database and the
	// failure surfaces as a node error only when a database node runs.
	maxConnections := uint32(1)
	acquireTimeout := uint64(500)
	engine, err := NewEngineWithDatabase(EngineConfig{}, "mysql://nobody@127.0.0.1:1/none", DatabaseOptions{
		MaxConnections: &maxConnections,
		AcquireTimeout: &acquireTimeout,
	})
	require.NoError(t, err)
	defer engine.Dispose()

	err = evaluateGraph(t, engine)
	require.Error(t, err)
	if DatabaseSupported {
		assert.NotContains(t, err.Error(), "database node not available")
	} else {
		assert.ErrorContains(t, err, "database node not available")
	}
}

func TestNewEngineWithDatabases_RejectsBadScheme(t *testing.T) {
	if !DatabaseSupported {
		t.Skip("database plug-in not linked on this platform")
	}
	_, err := NewEngineWithDatabases(EngineConfig{}, DatabaseConfig{
		Connections: map[string]DatabaseConnection{"a": {URL: "oracle://x"}},
	})
	assert.ErrorContains(t, err, "code 12")
}

func TestDatabaseNodeUnavailableHandler_DelegatesOtherKinds(t *testing.T) {
	called := false
	handler := DatabaseNodeUnavailableHandler(func(request NodeRequest) (NodeResponse, error) {
		called = true
		return NodeResponse{Output: map[string]any{"ok": true}}, nil
	})

	_, err := handler(NodeRequest{Node: CustomNode{Kind: "add"}})
	require.NoError(t, err)
	assert.True(t, called)

	_, err = handler(NodeRequest{Node: CustomNode{Kind: DatabaseNodeKind, Name: "db"}})
	assert.ErrorIs(t, err, ErrDatabaseNodeUnavailable)

	_, err = DatabaseNodeUnavailableHandler(nil)(NodeRequest{Node: CustomNode{Kind: "x"}})
	assert.ErrorContains(t, err, "unsupported custom node kind")
}

// maxRows, maxLoopItems and queryTimeout must reach the plug-in: the plug-in
// rejects unknown fields, so a wrong JSON name fails engine creation (code 12).
func TestDatabaseConnection_LimitsReachThePlugin(t *testing.T) {
	maxRows := uint32(5)
	maxLoopItems := uint32(20)
	queryTimeout := uint64(2000)
	config := DatabaseConfig{
		Connections: map[string]DatabaseConnection{
			"a": {URL: "mysql://nobody@127.0.0.1:1/none", MaxRows: &maxRows, MaxLoopItems: &maxLoopItems, QueryTimeout: &queryTimeout},
		},
	}

	encoded, err := config.toJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"connections":{"a":{"url":"mysql://nobody@127.0.0.1:1/none","maxRows":5,"maxLoopItems":20,"queryTimeout":2000}}}`, encoded)

	engine, err := NewEngineWithDatabases(EngineConfig{}, config)
	require.NoError(t, err)
	engine.Dispose()
}

func TestNewEngineWithDatabases_ConfigErrorCode(t *testing.T) {
	if !DatabaseSupported {
		t.Skip("database plug-in not linked on this platform")
	}
	_, err := NewEngineWithDatabases(EngineConfig{}, DatabaseConfig{
		Connections: map[string]DatabaseConnection{"a": {URL: "oracle://x"}},
	})
	assert.True(t, IsErrorCode(err, ErrorCodeDatabaseConfigError), "got %v", err)
}

func TestDatabaseConnectionSpeedOptionsEncodeAsThePluginReadsThem(t *testing.T) {
	// The plug-in rejects unknown keys (deny_unknown_fields), so the names must match.
	off, on := false, true
	config := DatabaseConfig{Connections: map[string]DatabaseConnection{"sr": {
		URL:               "starrocks://u@h:9030/db",
		TestBeforeAcquire: &off,
		SessionVariables:  map[string]string{"enable_short_circuit": "true"},
		Dedupe:            &on,
	}}}
	text, err := config.toJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"connections":{"sr":{"url":"starrocks://u@h:9030/db","testBeforeAcquire":false,
		"sessionVariables":{"enable_short_circuit":"true"},"dedupe":true}}}`, text)

	// unset options are left out, so the plug-in defaults apply
	text, err = DatabaseConfig{Connections: map[string]DatabaseConnection{"sr": {URL: "mysql://u@h/db"}}}.toJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"connections":{"sr":{"url":"mysql://u@h/db"}}}`, text)

	if DatabaseSupported {
		engine, err := NewEngineWithDatabases(EngineConfig{}, config)
		require.NoError(t, err, "the linked plug-in accepts every option")
		engine.Dispose()
	}
}
