package zen

// #include <stdlib.h>
// #include <string.h>
import "C"
import (
	"io"
	"runtime"
	"sync"
	"unsafe"
)

// RawResult is an engine response JSON that stays in the engine's memory. It
// reads like a bytes.Reader over that buffer; after Close it reads as empty.
// Close it when the bytes are no longer needed. A RawResult that is never closed
// is freed when it is garbage collected.
type RawResult struct {
	mu     sync.Mutex
	data   *C.char
	size   int
	offset int
}

// newRawResult takes ownership of data, which must not be nil.
func newRawResult(data *C.char) *RawResult {
	result := &RawResult{data: data, size: int(C.strlen(data))}
	runtime.SetFinalizer(result, (*RawResult).Close)
	return result
}

// Len is the size of the JSON in bytes.
func (result *RawResult) Len() int {
	result.mu.Lock()
	defer result.mu.Unlock()
	return result.size
}

// Read implements io.Reader over the JSON.
func (result *RawResult) Read(b []byte) (int, error) {
	result.mu.Lock()
	defer result.mu.Unlock()
	if result.data == nil || result.offset >= result.size {
		return 0, io.EOF
	}
	copied := copy(b, cView(result.data, result.size)[result.offset:])
	result.offset += copied
	return copied, nil
}

// LastByte is the final byte of the JSON (false when it is empty or closed).
func (result *RawResult) LastByte() (byte, bool) {
	result.mu.Lock()
	defer result.mu.Unlock()
	if result.data == nil || result.size == 0 {
		return 0, false
	}
	return cView(result.data, result.size)[result.size-1], true
}

// Bytes copies the JSON into Go memory.
func (result *RawResult) Bytes() []byte {
	result.mu.Lock()
	defer result.mu.Unlock()
	if result.data == nil {
		return nil
	}
	return goBytes(result.data, result.size)
}

// Close frees the engine's buffer; later calls do nothing.
func (result *RawResult) Close() error {
	result.mu.Lock()
	defer result.mu.Unlock()
	if result.data != nil {
		C.free(unsafe.Pointer(result.data))
		result.data, result.size, result.offset = nil, 0, 0
		runtime.SetFinalizer(result, nil)
	}
	return nil
}
