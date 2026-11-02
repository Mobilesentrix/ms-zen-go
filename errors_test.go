package zen_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mobilesentrix/ms-zen-go/v2"
)

func TestErrorCode_Names(t *testing.T) {
	tests := []struct {
		code zen.ErrorCode
		name string
	}{
		{zen.ErrorCodeInvalidArgument, "InvalidArgument"},
		{zen.ErrorCodeEvaluationError, "EvaluationError"},
		{zen.ErrorCodeLoaderKeyNotFound, "LoaderKeyNotFound"},
		{zen.ErrorCodeLoaderConfigError, "LoaderConfigError"},
		{zen.ErrorCodeDatabaseConfigError, "DatabaseConfigError"},
		{zen.ErrorCodePanic, "Panic"},
		{zen.ErrorCode(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.name, tt.code.String())
	}
	assert.Equal(t, zen.ErrorCode(13), zen.ErrorCodePanic)
	assert.Equal(t, zen.ErrorCode(12), zen.ErrorCodeDatabaseConfigError)
}

// Engine failures carry the FFI error code; the message is the engine's
// details string, unchanged, so callers that parse it keep working.
func TestEngine_EvaluationErrorCarriesCode(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{Loader: zen.Loader(readTestFile)})
	defer engine.Dispose()

	_, err := engine.Evaluate("does-not-exist.json", map[string]any{})
	require.Error(t, err)

	var zenErr *zen.Error
	require.True(t, errors.As(err, &zenErr), "got %T", err)
	assert.NotEqual(t, zen.ErrorCode(0), zenErr.Code)
	assert.Equal(t, zenErr.Details, err.Error())
	assert.True(t, zen.IsErrorCode(err, zenErr.Code))
	assert.False(t, zen.IsErrorCode(err, zen.ErrorCodePanic))
}
