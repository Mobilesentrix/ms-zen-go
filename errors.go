package zen

import (
	"errors"
	"fmt"
)

// ErrorCode is the error code a libzen_ffi call returns (ZenResult.error).
// 0–11 are upstream zen; 12 (the database connection map was rejected) and 13
// (a Rust panic was caught at the C boundary) are MobileSentrix additions.
type ErrorCode uint8

const (
	ErrorCodeInvalidArgument         ErrorCode = 1
	ErrorCodeStringNullError         ErrorCode = 2
	ErrorCodeStringUtf8Error         ErrorCode = 3
	ErrorCodeJsonSerializationFailed ErrorCode = 4
	ErrorCodeJsonDeserialization     ErrorCode = 5
	ErrorCodeIsolateError            ErrorCode = 6
	ErrorCodeEvaluationError         ErrorCode = 7
	ErrorCodeLoaderKeyNotFound       ErrorCode = 8
	ErrorCodeLoaderInternalError     ErrorCode = 9
	ErrorCodeTemplateEngineError     ErrorCode = 10
	ErrorCodeLoaderConfigError       ErrorCode = 11
	// ErrorCodeDatabaseConfigError: the database connection map was rejected.
	ErrorCodeDatabaseConfigError ErrorCode = 12
	// ErrorCodePanic: a Rust panic was caught at the C boundary. The engine
	// stays usable; the call that panicked failed.
	ErrorCodePanic ErrorCode = 13
)

var errorCodeNames = map[ErrorCode]string{
	ErrorCodeInvalidArgument:         "InvalidArgument",
	ErrorCodeStringNullError:         "StringNullError",
	ErrorCodeStringUtf8Error:         "StringUtf8Error",
	ErrorCodeJsonSerializationFailed: "JsonSerializationFailed",
	ErrorCodeJsonDeserialization:     "JsonDeserializationFailed",
	ErrorCodeIsolateError:            "IsolateError",
	ErrorCodeEvaluationError:         "EvaluationError",
	ErrorCodeLoaderKeyNotFound:       "LoaderKeyNotFound",
	ErrorCodeLoaderInternalError:     "LoaderInternalError",
	ErrorCodeTemplateEngineError:     "TemplateEngineError",
	ErrorCodeLoaderConfigError:       "LoaderConfigError",
	ErrorCodeDatabaseConfigError:     "DatabaseConfigError",
	ErrorCodePanic:                   "Panic",
}

func (code ErrorCode) String() string {
	if name, ok := errorCodeNames[code]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", uint8(code))
}

// Error is a failed libzen_ffi call. Error() returns the engine's details
// string unchanged (usually JSON such as {"type":"NodeError",...}), exactly as
// upstream's plain errors, so callers that parse it keep working.
type Error struct {
	Code    ErrorCode
	Details string
}

func (err *Error) Error() string {
	return err.Details
}

// IsErrorCode reports whether err (or an error it wraps) is an engine Error
// with the given code.
func IsErrorCode(err error, code ErrorCode) bool {
	var zenErr *Error
	return errors.As(err, &zenErr) && zenErr.Code == code
}
