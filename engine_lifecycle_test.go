package zen_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mobilesentrix/ms-zen-go/v2"
)

// A panic inside a Go callback must never unwind into the Rust frames that
// called it (zen_engine_evaluate -> tokio block_on -> Go export). It has to be
// turned into a node/loader error and the engine must stay usable.
func TestEngine_CustomNodePanicBecomesError(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{
		Loader: zen.Loader(readTestFile),
		CustomNodeHandler: func(zen.NodeRequest) (zen.NodeResponse, error) {
			panic("custom node handler bug")
		},
	})
	defer engine.Dispose()

	_, err := engine.Evaluate("custom-node.json", map[string]any{"a": 5, "b": 10})
	require.Error(t, err)
	assert.ErrorContains(t, err, "panic")

	output, err := engine.Evaluate("table.json", map[string]any{"input": 15})
	require.NoError(t, err, "engine must survive a panicking handler")
	assert.JSONEq(t, `{"output":10}`, string(output.Result))
}

func TestEngine_LoaderPanicBecomesError(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{
		Loader: zen.Loader(func(key string) ([]byte, error) {
			if key == "table.json" {
				return readTestFile(key)
			}
			panic("loader exploded on " + key)
		}),
	})
	defer engine.Dispose()

	_, err := engine.Evaluate("missing.json", map[string]any{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "loader exploded on missing.json")

	output, err := engine.Evaluate("table.json", map[string]any{"input": 15})
	require.NoError(t, err)
	assert.JSONEq(t, `{"output":10}`, string(output.Result))
}

// Dispose twice must be a no-op (no double zen_engine_free, no cgo.Handle
// double-Delete panic), and every call after Dispose must fail cleanly instead
// of using a freed engine pointer.
func TestEngine_DisposeIsIdempotentAndFencesLaterCalls(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile), CustomNodeHandler: customNodeHandler})

	output, err := engine.Evaluate("table.json", map[string]any{"input": 15})
	require.NoError(t, err)
	assert.JSONEq(t, `{"output":10}`, string(output.Result))

	engine.Dispose()
	assert.NotPanics(t, engine.Dispose)

	_, err = engine.Evaluate("table.json", map[string]any{"input": 15})
	assert.ErrorIs(t, err, zen.ErrEngineDisposed)

	_, err = engine.EvaluateBatch([]zen.EvaluateBatchRequest{{Key: "table.json", Context: map[string]any{"input": 15}}})
	assert.ErrorIs(t, err, zen.ErrEngineDisposed)

	_, err = engine.GetDecision("table.json")
	assert.ErrorIs(t, err, zen.ErrEngineDisposed)

	_, err = engine.CreateDecision([]byte(`{"nodes":[],"edges":[]}`))
	assert.ErrorIs(t, err, zen.ErrEngineDisposed)

	assert.NotPanics(t, engine.Dispose)
}

func TestEngine_DisposeWaitsForInFlightEvaluations(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile), CustomNodeHandler: customNodeHandler})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := engine.Evaluate("function.json", map[string]any{"input": i})
			// Either the evaluation ran before Dispose, or it was fenced; it must
			// never touch a freed engine (the race detector / ASan would flag that).
			if err != nil {
				assert.ErrorIs(t, err, zen.ErrEngineDisposed)
			}
		}(i)
	}
	engine.Dispose()
	wg.Wait()
}

func TestDecision_DisposeIsIdempotentAndFencesLaterCalls(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	defer engine.Dispose()

	decision, err := engine.GetDecision("table.json")
	require.NoError(t, err)

	output, err := decision.Evaluate(map[string]any{"input": 15})
	require.NoError(t, err)
	assert.JSONEq(t, `{"output":10}`, string(output.Result))

	decision.Dispose()
	assert.NotPanics(t, decision.Dispose)

	_, err = decision.Evaluate(map[string]any{"input": 15})
	assert.ErrorIs(t, err, zen.ErrDecisionDisposed)
}

func TestEngine_InitErrorIsReturnedAndDisposeIsSafe(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.ZipLoader{}})
	_, err := engine.Evaluate("table.json", nil)
	require.Error(t, err)
	assert.NotPanics(t, engine.Dispose)
	assert.NotPanics(t, engine.Dispose)
}

// C strings end at the first NUL byte: a raw JSON payload containing one would be
// silently truncated before it reached the engine. Reject it instead.
func TestEngine_RejectsNulBytes(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	defer engine.Dispose()

	_, err := engine.Evaluate("table.json", []byte("{\"input\": 15}\x00, \"x\": 1}"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "NUL")

	_, err = engine.Evaluate("table.json\x00", map[string]any{"input": 15})
	require.Error(t, err)
	assert.ErrorContains(t, err, "NUL")

	_, err = engine.EvaluateBatch([]zen.EvaluateBatchRequest{
		{Key: "table.json", Context: map[string]any{"input": 15}},
		{Key: "table.json", Context: []byte("{\x00}")},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "NUL")

	_, err = engine.CreateDecision([]byte("{\"nodes\":[]\x00}"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "NUL")

	decision, err := engine.GetDecision("table.json")
	require.NoError(t, err)
	defer decision.Dispose()
	_, err = decision.Evaluate([]byte("{\x00}"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "NUL")

	// A marshalled value is always safe: encoding/json escapes U+0000 as \u0000.
	output, err := engine.Evaluate("table.json", map[string]any{"input": 15, "note": "a\x00b"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"output":10}`, string(output.Result))
}

func TestDatabaseNodeUnavailableHandler_PanicInNextBecomesError(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{
		Loader: zen.Loader(readTestFile),
		CustomNodeHandler: zen.DatabaseNodeUnavailableHandler(func(zen.NodeRequest) (zen.NodeResponse, error) {
			panic(errors.New("next handler panicked"))
		}),
	})
	defer engine.Dispose()

	_, err := engine.Evaluate("custom-node.json", map[string]any{"a": 5, "b": 10})
	require.Error(t, err)
	assert.ErrorContains(t, err, "next handler panicked")
}

// Dispose waits for an evaluation that is still inside the engine: the loader
// below blocks until released, and Dispose must not return (nor free the engine)
// before that evaluation has finished.
func TestEngine_DisposeWaitsForAnInFlightCall(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(func(key string) ([]byte, error) {
		once.Do(func() { close(entered) })
		<-release
		return readTestFile(key)
	})})

	evaluated := make(chan error, 1)
	go func() {
		_, err := engine.Evaluate("table.json", map[string]any{"input": 15})
		evaluated <- err
	}()
	<-entered

	disposed := make(chan struct{})
	go func() {
		engine.Dispose()
		close(disposed)
	}()
	select {
	case <-disposed:
		t.Fatal("Dispose returned while an evaluation was still running")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-evaluated)
	select {
	case <-disposed:
	case <-time.After(5 * time.Second):
		t.Fatal("Dispose did not return after the evaluation finished")
	}
	_, err := engine.Evaluate("table.json", map[string]any{"input": 15})
	assert.ErrorIs(t, err, zen.ErrEngineDisposed)
}
