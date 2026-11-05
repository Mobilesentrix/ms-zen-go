package zen

// #include "zen_engine.h"
import "C"

// Loader returns the content of decision key. It runs inside the engine call
// that needs the decision: it must not call Dispose on that engine.
type Loader func(key string) ([]byte, error)

func (Loader) engineLoader() {}

func wrapLoader(loader Loader) func(cKey *C.char) C.ZenDecisionLoaderResult {
	return func(cKey *C.char) C.ZenDecisionLoaderResult {
		key := C.GoString(cKey)
		content, err := loader(key)
		if err != nil {
			return C.ZenDecisionLoaderResult{
				content: nil,
				error:   C.CString(err.Error()),
			}
		}

		return C.ZenDecisionLoaderResult{
			content: C.CString(string(content)),
			error:   nil,
		}
	}
}
