package zen

import "encoding/json"

type EvaluationOptions struct {
	Trace    bool  `json:"trace"`
	MaxDepth uint8 `json:"maxDepth"`
}

type EvaluationResponse struct {
	Performance string           `json:"performance"`
	Result      json.RawMessage  `json:"result"`
	Trace       *json.RawMessage `json:"trace"`
}

type EvaluateBatchRequest struct {
	Key     string
	Context any
}

type EvaluateBatchResult struct {
	Success bool                `json:"success"`
	Data    *EvaluationResponse `json:"data,omitempty"`
	Error   json.RawMessage     `json:"error,omitempty"`
}

type Engine interface {
	Evaluate(key string, context any) (*EvaluationResponse, error)
	EvaluateWithOpts(key string, context any, options EvaluationOptions) (*EvaluationResponse, error)
	EvaluateBatch(requests []EvaluateBatchRequest) ([]EvaluateBatchResult, error)
	EvaluateBatchWithOpts(requests []EvaluateBatchRequest, options EvaluationOptions) ([]EvaluateBatchResult, error)
	GetDecision(key string) (Decision, error)
	CreateDecision(data []byte) (Decision, error)
	Dispose()
}

// DecisionCache is implemented by the Engine NewEngine and NewEngineWithDatabase(s)
// return (type-assert it, like RawEvaluator). The engine compiles each decision
// its loader returns once and keeps it; tell it when the loader's content changes.
type DecisionCache interface {
	// InvalidateDecision drops the compiled decision cached for key; the next
	// evaluation of key (directly or as a sub-decision) reloads it.
	InvalidateDecision(key string) error
	// ClearDecisions drops every cached decision (full rule-set reload).
	ClearDecisions() error
}

type Decision interface {
	Evaluate(context any) (*EvaluationResponse, error)
	EvaluateWithOpts(context any, options EvaluationOptions) (*EvaluationResponse, error)
	Dispose()
}
