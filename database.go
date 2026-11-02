package zen

import (
	"encoding/json"
	"errors"
	"fmt"
)

// DatabaseNodeKind is the custom-node kind of a database fetch:
//
//	{"type": "customNode", "content": {"kind": DatabaseNodeKind, "config": {...}}}
//
// The config holds `query`, `outputPath`, `passThrough` and optionally `instance`.
// With DatabaseSupported and an engine from NewEngineWithDatabase(s) these nodes
// run natively in Rust. Otherwise they reach the Go custom-node handler and fail
// with ErrDatabaseNodeUnavailable.
const DatabaseNodeKind = "database"

// DefaultDatabaseConnection is the connection name NewEngineWithDatabase uses for
// its single URL.
const DefaultDatabaseConnection = "default"

// ErrDatabaseNodeUnavailable is returned for database nodes when the engine has
// no database plug-in.
var ErrDatabaseNodeUnavailable = errors.New("database node not available: this engine has no database plug-in or no database connection configured")

// DatabaseOptions configures one connection pool. Timeouts are milliseconds.
type DatabaseOptions struct {
	MaxConnections *uint32 `json:"maxConnections,omitempty"`
	MinConnections *uint32 `json:"minConnections,omitempty"`
	AcquireTimeout *uint64 `json:"acquireTimeout,omitempty"`
	IdleTimeout    *uint64 `json:"idleTimeout,omitempty"`
	MaxLifetime    *uint64 `json:"maxLifetime,omitempty"`
}

// DatabaseConnection is one named pool. URL schemes: mysql, mariadb, starrocks,
// postgres, postgresql.
type DatabaseConnection struct {
	URL     string   `json:"url"`
	Aliases []string `json:"aliases,omitempty"`
	DatabaseOptions
	StatementCacheCapacity *uint32 `json:"statementCacheCapacity,omitempty"`
	// ConnectEagerly opens one connection at engine creation and fails creation
	// if the database is unreachable. Default: pools connect lazily.
	ConnectEagerly bool `json:"connectEagerly,omitempty"`
	// MaxRows fails a query that returns more rows than this (plug-in default
	// 10000, minimum 1).
	MaxRows *uint32 `json:"maxRows,omitempty"`
	// MaxLoopItems fails a loop-mode database node over more items than this
	// (plug-in default 1000, minimum 1).
	MaxLoopItems *uint32 `json:"maxLoopItems,omitempty"`
	// QueryTimeout bounds one statement's execution, in milliseconds (no
	// default). AcquireTimeout only bounds obtaining a connection.
	QueryTimeout *uint64 `json:"queryTimeout,omitempty"`
	// TestBeforeAcquire pings a pooled connection before each query (plug-in
	// default true). When it is false, keep IdleTimeout below every idle
	// disconnect on the path (server wait_timeout, load-balancer idle timeout).
	TestBeforeAcquire *bool `json:"testBeforeAcquire,omitempty"`
	// SessionVariables are `SET name = value` statements run on every new
	// connection, e.g. StarRocks {"enable_short_circuit": "true"}. Names must be
	// identifiers; the plug-in rejects anything else.
	SessionVariables map[string]string `json:"sessionVariables,omitempty"`
	// Dedupe lets identical queries inside one evaluation share a round trip
	// (plug-in default true). Rows are never kept beyond the evaluation.
	Dedupe *bool `json:"dedupe,omitempty"`
}

// DatabaseConfig is the engine's database plug-in configuration. Connection
// names are what a node's `instance` refers to; nodes without `instance` use
// Default (or the only connection when there is exactly one).
type DatabaseConfig struct {
	Connections   map[string]DatabaseConnection `json:"connections"`
	Default       string                        `json:"default,omitempty"`
	WorkerThreads *uint32                       `json:"workerThreads,omitempty"`
}

func (c DatabaseConfig) validate() error {
	if c.Connections == nil {
		return errors.New("zen-engine: database config requires connections (may be empty)")
	}
	for name, conn := range c.Connections {
		if conn.URL == "" {
			return fmt.Errorf("zen-engine: database connection %q has no URL", name)
		}
	}
	if c.Default != "" {
		if _, ok := c.Connections[c.Default]; !ok {
			return fmt.Errorf("zen-engine: default database connection %q is not configured", c.Default)
		}
	}
	return nil
}

func (c DatabaseConfig) toJSON() (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("zen-engine: encode database config: %w", err)
	}
	return string(data), nil
}

// NewEngineWithDatabase creates an engine with one database connection, named
// DefaultDatabaseConnection, that serves every database node.
func NewEngineWithDatabase(config EngineConfig, databaseURL string, options DatabaseOptions) (Engine, error) {
	if databaseURL == "" {
		return nil, errors.New("zen-engine: database URL is required")
	}
	return NewEngineWithDatabases(config, DatabaseConfig{
		Connections: map[string]DatabaseConnection{
			DefaultDatabaseConnection: {URL: databaseURL, DatabaseOptions: options},
		},
		Default: DefaultDatabaseConnection,
	})
}

// DatabaseNodeUnavailableHandler wraps next so that database nodes fail with
// ErrDatabaseNodeUnavailable instead of the engine's generic "custom node handler
// not provided" message. Other kinds go to next; when next is nil they fail with
// an explicit unsupported-kind error.
func DatabaseNodeUnavailableHandler(next CustomNodeHandler) CustomNodeHandler {
	return func(request NodeRequest) (NodeResponse, error) {
		if request.Node.Kind == DatabaseNodeKind {
			return NodeResponse{}, fmt.Errorf("node %q: %w", request.Node.Name, ErrDatabaseNodeUnavailable)
		}
		if next == nil {
			return NodeResponse{}, fmt.Errorf("node %q: unsupported custom node kind %q", request.Node.Name, request.Node.Kind)
		}
		return next(request)
	}
}
