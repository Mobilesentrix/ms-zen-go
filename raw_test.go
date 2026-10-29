package zen_test

import (
	"encoding/json"
	"io"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mobilesentrix/ms-zen-go/v2"
)

// The raw path takes the context JSON bytes as the client sent them and
// returns the engine's response bytes as is: no Go map, no re-encoding.

func rawEngine(t *testing.T) (zen.Engine, zen.RawEvaluator) {
	t.Helper()
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile), CustomNodeHandler: customNodeHandler})
	t.Cleanup(engine.Dispose)
	raw, ok := engine.(zen.RawEvaluator)
	require.True(t, ok, "the engine implements RawEvaluator")
	return engine, raw
}

func TestEngine_EvaluateRawMatchesEvaluate(t *testing.T) {
	engine, raw := rawEngine(t)
	for _, data := range prepareEvaluationTestData() {
		var input any
		require.NoError(t, json.Unmarshal([]byte(data.inputJson), &input))
		want, err := engine.EvaluateWithOpts(data.file, input, zen.EvaluationOptions{MaxDepth: 10})
		require.NoError(t, err)

		got, err := raw.EvaluateRaw(data.file, []byte(data.inputJson), zen.EvaluationOptions{MaxDepth: 10})
		require.NoError(t, err, data.file)
		var resp zen.EvaluationResponse
		require.NoError(t, json.Unmarshal(got, &resp), string(got))
		assert.JSONEq(t, string(want.Result), string(resp.Result), data.file)
		assert.JSONEq(t, data.outputJson, string(resp.Result), data.file)
		assert.Nil(t, resp.Trace, "no trace key when not traced")
	}
}

func TestEngine_EvaluateRawTraced(t *testing.T) {
	_, raw := rawEngine(t)
	data := prepareEvaluationTestData()["table > 10"]
	got, err := raw.EvaluateRaw(data.file, []byte(data.inputJson), zen.EvaluationOptions{Trace: true, MaxDepth: 10})
	require.NoError(t, err)
	var resp zen.EvaluationResponse
	require.NoError(t, json.Unmarshal(got, &resp))
	assert.NotNil(t, resp.Trace)
}

func TestEngine_EvaluateRawRejectsNUL(t *testing.T) {
	_, raw := rawEngine(t)
	_, err := raw.EvaluateRaw("table.json", []byte("{\"a\":1}\x00"), zen.EvaluationOptions{})
	assert.ErrorContains(t, err, "NUL")
	_, err = raw.EvaluateRaw("table\x00.json", []byte(`{}`), zen.EvaluationOptions{})
	assert.ErrorContains(t, err, "NUL")
}

func TestEngine_EvaluateRawErrorsLikeEvaluate(t *testing.T) {
	engine, raw := rawEngine(t)
	_, wantErr := engine.Evaluate("missing.json", map[string]any{})
	_, gotErr := raw.EvaluateRaw("missing.json", []byte(`{}`), zen.EvaluationOptions{})
	require.Error(t, wantErr)
	assert.Equal(t, wantErr.Error(), gotErr.Error())
}

// json.RawMessage contexts are passed through like []byte (no re-encode).
func TestEngine_EvaluateRawMessageContext(t *testing.T) {
	engine, _ := rawEngine(t)
	data := prepareEvaluationTestData()["table > 10"]
	out, err := engine.Evaluate(data.file, json.RawMessage(data.inputJson))
	require.NoError(t, err)
	assert.JSONEq(t, data.outputJson, string(out.Result))
}

func TestDecision_EvaluateRaw(t *testing.T) {
	engine, _ := rawEngine(t)
	data := prepareEvaluationTestData()["table > 10"]
	content, err := readTestFile(data.file)
	require.NoError(t, err)
	d, err := engine.CreateDecision(content)
	require.NoError(t, err)
	defer d.Dispose()
	raw, ok := d.(zen.RawDecision)
	require.True(t, ok)
	got, err := raw.EvaluateRaw([]byte(data.inputJson), zen.EvaluationOptions{Trace: true, MaxDepth: 10})
	require.NoError(t, err)
	var resp zen.EvaluationResponse
	require.NoError(t, json.Unmarshal(got, &resp))
	assert.JSONEq(t, data.outputJson, string(resp.Result))
	assert.NotNil(t, resp.Trace)
}

func BenchmarkEvaluateMap(b *testing.B) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile), CustomNodeHandler: customNodeHandler})
	defer engine.Dispose()
	data := prepareEvaluationTestData()["table > 10"]
	var input map[string]any
	_ = json.Unmarshal([]byte(data.inputJson), &input)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.EvaluateWithOpts(data.file, input, zen.EvaluationOptions{MaxDepth: 10}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluateRaw(b *testing.B) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile), CustomNodeHandler: customNodeHandler})
	defer engine.Dispose()
	raw := engine.(zen.RawEvaluator)
	data := prepareEvaluationTestData()["table > 10"]
	ctx := []byte(data.inputJson)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := raw.EvaluateRaw(data.file, ctx, zen.EvaluationOptions{MaxDepth: 10}); err != nil {
			b.Fatal(err)
		}
	}
}

// EvaluateResult hands out the same bytes as EvaluateRaw without copying them
// into Go memory; reading in small pieces, Len, and Close (twice) behave as a
// closed reader should.
func TestEngine_EvaluateResultReadsLikeEvaluateRaw(t *testing.T) {
	engine, raw := rawEngine(t)
	res, ok := engine.(zen.ResultEvaluator)
	require.True(t, ok, "the engine implements ResultEvaluator")
	data := prepareEvaluationTestData()["table > 10"]
	for _, trace := range []bool{false, true} {
		opts := zen.EvaluationOptions{Trace: trace, MaxDepth: 10}
		want, err := raw.EvaluateRaw(data.file, []byte(data.inputJson), opts)
		require.NoError(t, err)

		r, err := res.EvaluateResult(data.file, []byte(data.inputJson), opts)
		require.NoError(t, err)
		size := r.Len()
		last, ok := r.LastByte()
		require.True(t, ok)
		assert.Equal(t, want[len(want)-1], last)
		var got []byte
		buf := make([]byte, 7)
		for {
			n, err := r.Read(buf)
			got = append(got, buf[:n]...)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		assert.Equal(t, size, len(got))
		assertSameExceptPerformance(t, want, got)
		require.NoError(t, r.Close())
		require.NoError(t, r.Close())
		n, err := r.Read(buf)
		assert.Equal(t, 0, n)
		assert.Equal(t, io.EOF, err)
		assert.Nil(t, r.Bytes())
		_, ok = r.LastByte()
		assert.False(t, ok)
	}
}

func TestDecision_EvaluateResult(t *testing.T) {
	engine, _ := rawEngine(t)
	data := prepareEvaluationTestData()["table > 10"]
	content, err := readTestFile(data.file)
	require.NoError(t, err)
	decision, err := engine.CreateDecision(content)
	require.NoError(t, err)
	t.Cleanup(decision.Dispose)
	res, ok := decision.(zen.ResultDecision)
	require.True(t, ok, "a decision implements ResultDecision")
	r, err := res.EvaluateResult([]byte(data.inputJson), zen.EvaluationOptions{MaxDepth: 10})
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	var resp zen.EvaluationResponse
	require.NoError(t, json.Unmarshal(got, &resp), string(got))
	assert.JSONEq(t, data.outputJson, string(resp.Result))
}

// assertSameExceptPerformance compares two responses of one evaluation; the
// "performance" timings (top level and per traced node) are the only part
// that may differ.
func assertSameExceptPerformance(t *testing.T, want, got []byte) {
	t.Helper()
	timing := regexp.MustCompile(`"performance":"[^"]*"`)
	assert.Equal(t, string(timing.ReplaceAll(want, nil)), string(timing.ReplaceAll(got, nil)))
}
