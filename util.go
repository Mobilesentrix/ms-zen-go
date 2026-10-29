package zen

import "encoding/json"

func extractJsonFromAny(data any) (json.RawMessage, error) {
	return contextJSON(data)
}
