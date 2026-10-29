package zen_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mobilesentrix/ms-zen-go/v2"
)

// versionGraph returns a decision whose result is {"version": <version>}.
func versionGraph(version int) []byte {
	return []byte(fmt.Sprintf(`{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request", "position": {"x": 0, "y": 0}},
    {"id": "ex", "type": "expressionNode", "name": "Version", "position": {"x": 0, "y": 0},
     "content": {"expressions": [{"id": "v", "key": "version", "value": "%d"}]}},
    {"id": "out", "type": "outputNode", "name": "Response", "position": {"x": 0, "y": 0}}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "ex", "type": "edge"},
    {"id": "e2", "sourceId": "ex", "targetId": "out", "type": "edge"}
  ]
}`, version))
}

// parentGraph calls the decision `child` through a decision node.
func parentGraph(child string) []byte {
	return []byte(fmt.Sprintf(`{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request", "position": {"x": 0, "y": 0}},
    {"id": "sub", "type": "decisionNode", "name": "Child", "position": {"x": 0, "y": 0},
     "content": {"key": %q}},
    {"id": "out", "type": "outputNode", "name": "Response", "position": {"x": 0, "y": 0}}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "sub", "type": "edge"},
    {"id": "e2", "sourceId": "sub", "targetId": "out", "type": "edge"}
  ]
}`, child))
}

// mutableLoader is a loader whose content can change between evaluations, like
// module-go's RulesStorage after a save or an S3 refresh.
type mutableLoader struct {
	mu    sync.RWMutex
	rules map[string][]byte
	loads atomic.Int64
}

func newMutableLoader(rules map[string][]byte) *mutableLoader {
	return &mutableLoader{rules: rules}
}

func (l *mutableLoader) set(key string, content []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rules[key] = content
}

func (l *mutableLoader) load(key string) ([]byte, error) {
	l.loads.Add(1)
	l.mu.RLock()
	defer l.mu.RUnlock()
	content, ok := l.rules[key]
	if !ok {
		return nil, errors.New("not found: " + key)
	}
	return content, nil
}

func evaluateVersion(t *testing.T, engine zen.Engine, key string) string {
	t.Helper()
	response, err := engine.EvaluateWithOpts(key, map[string]any{}, zen.EvaluationOptions{MaxDepth: 10})
	require.NoError(t, err)
	return string(response.Result)
}

// The engine compiles each key once (MS-FFI.md "Decision cache"): a changed
// rule keeps being served from the cache until it is invalidated.
func TestEngine_InvalidateDecisionReloadsOneKey(t *testing.T) {
	loader := newMutableLoader(map[string][]byte{"a": versionGraph(1), "b": versionGraph(10)})
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(loader.load)})
	defer engine.Dispose()

	assert.JSONEq(t, `{"version":1}`, evaluateVersion(t, engine, "a"))
	assert.JSONEq(t, `{"version":10}`, evaluateVersion(t, engine, "b"))
	loadsAfterWarmup := loader.loads.Load()

	loader.set("a", versionGraph(2))
	loader.set("b", versionGraph(20))
	assert.JSONEq(t, `{"version":1}`, evaluateVersion(t, engine, "a"), "cached until invalidated")
	assert.Equal(t, loadsAfterWarmup, loader.loads.Load(), "cached keys must not hit the loader")

	require.NoError(t, cache(t, engine).InvalidateDecision("a"))
	assert.JSONEq(t, `{"version":2}`, evaluateVersion(t, engine, "a"))
	assert.JSONEq(t, `{"version":10}`, evaluateVersion(t, engine, "b"), "other keys stay cached")
}

func TestEngine_ClearDecisionsReloadsEveryKey(t *testing.T) {
	loader := newMutableLoader(map[string][]byte{"a": versionGraph(1), "b": versionGraph(10)})
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(loader.load)})
	defer engine.Dispose()

	evaluateVersion(t, engine, "a")
	evaluateVersion(t, engine, "b")

	loader.set("a", versionGraph(2))
	loader.set("b", versionGraph(20))
	require.NoError(t, cache(t, engine).ClearDecisions())

	assert.JSONEq(t, `{"version":2}`, evaluateVersion(t, engine, "a"))
	assert.JSONEq(t, `{"version":20}`, evaluateVersion(t, engine, "b"))
}

// A parent reaches its sub-decision through the loader, so invalidating only
// the child is enough for the parent to see the new child.
func TestEngine_InvalidateChildIsSeenByParent(t *testing.T) {
	loader := newMutableLoader(map[string][]byte{"parent": parentGraph("child"), "child": versionGraph(1)})
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(loader.load)})
	defer engine.Dispose()

	assert.JSONEq(t, `{"version":1}`, evaluateVersion(t, engine, "parent"))

	loader.set("child", versionGraph(2))
	require.NoError(t, cache(t, engine).InvalidateDecision("child"))
	assert.JSONEq(t, `{"version":2}`, evaluateVersion(t, engine, "parent"))
}

func TestEngine_InvalidateUnknownKeyIsNoop(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	defer engine.Dispose()

	assert.NoError(t, cache(t, engine).InvalidateDecision("never-loaded.json"))
	assert.NoError(t, cache(t, engine).ClearDecisions())
}

func TestEngine_InvalidateRejectsNulKey(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	defer engine.Dispose()

	assert.ErrorContains(t, cache(t, engine).InvalidateDecision("a\x00b"), "NUL")
}

func TestEngine_CacheCallsAfterDispose(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	engine.Dispose()

	assert.ErrorIs(t, cache(t, engine).InvalidateDecision("table.json"), zen.ErrEngineDisposed)
	assert.ErrorIs(t, cache(t, engine).ClearDecisions(), zen.ErrEngineDisposed)
}

// Invalidation while other goroutines evaluate must neither crash nor serve a
// torn decision: every result is one of the two published versions.
func TestEngine_InvalidateDuringConcurrentEvaluation(t *testing.T) {
	loader := newMutableLoader(map[string][]byte{"a": versionGraph(1)})
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(loader.load)})
	defer engine.Dispose()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				response, err := engine.Evaluate("a", map[string]any{})
				if !assert.NoError(t, err) {
					return
				}
				result := string(response.Result)
				assert.Contains(t, []string{`{"version":1}`, `{"version":2}`}, result)
			}
		}()
	}

	loader.set("a", versionGraph(2))
	for i := 0; i < 50; i++ {
		require.NoError(t, cache(t, engine).InvalidateDecision("a"))
	}
	wg.Wait()

	assert.JSONEq(t, `{"version":2}`, evaluateVersion(t, engine, "a"))
}

// cache is the engine's DecisionCache (an optional interface of zen.Engine).
func cache(t *testing.T, engine zen.Engine) zen.DecisionCache {
	t.Helper()
	c, ok := engine.(zen.DecisionCache)
	require.True(t, ok, "the engine implements zen.DecisionCache")
	return c
}
