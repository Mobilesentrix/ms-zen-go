package zen_test

import (
	"encoding/json"
	"testing"

	"github.com/Mobilesentrix/ms-zen-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The list functions and the sequential execution mode come from the zen fork library in deps/.
// These tests fail on a library built without them.

func TestExpression_ListFunctions(t *testing.T) {
	ctx := map[string]any{
		"items": []any{
			map[string]any{"sku": "b", "qty": 3},
			map[string]any{"sku": "a", "qty": 1},
			map[string]any{"sku": "b", "qty": 2},
		},
	}

	cases := []struct {
		expression string
		want       string
	}{
		{"map(sortBy(items, #.sku), #.qty)", `[1, 3, 2]`},
		{"reduce(items, acc + #.qty, 0)", `6`},
		{"unique(map(items, #.sku))", `["b", "a"]`},
	}

	for _, c := range cases {
		t.Run(c.expression, func(t *testing.T) {
			got, err := zen.EvaluateExpression[any](c.expression, ctx)
			require.NoError(t, err)
			gotJSON, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, c.want, string(gotJSON))
		})
	}
}

const sequentialGraph = `{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request", "content": {}},
    {
      "id": "allocate", "type": "expressionNode", "name": "Allocate budget",
      "content": {
        "expressions": [
          {"id": "e1", "key": "warehouseId", "value": "id"},
          {"id": "e2", "key": "take", "value": "min([need, state.remaining])"},
          {"id": "e3", "key": "state.remaining", "value": "state.remaining - min([need, state.remaining])"}
        ],
        "inputField": "warehouses",
        "executionMode": "sequential",
        "stateField": "{remaining: budget}",
        "outputPath": "allocations"
      }
    },
    {"id": "out", "type": "outputNode", "name": "Response", "content": {}}
  ],
  "edges": [
    {"id": "a", "sourceId": "in", "targetId": "allocate", "type": "edge"},
    {"id": "b", "sourceId": "allocate", "targetId": "out", "type": "edge"}
  ]
}`

func TestDecision_SequentialStep(t *testing.T) {
	engine := zen.NewEngine(zen.EngineConfig{})
	defer engine.Dispose()

	decision, err := engine.CreateDecision([]byte(sequentialGraph))
	require.NoError(t, err)
	defer decision.Dispose()

	input := map[string]any{
		"budget": 100,
		"warehouses": []any{
			map[string]any{"id": 1, "need": 30},
			map[string]any{"id": 2, "need": 50.5},
			map[string]any{"id": 3, "need": 40},
			map[string]any{"id": 4, "need": 10},
		},
	}

	output, err := decision.Evaluate(input)
	require.NoError(t, err)
	result, err := output.Result.MarshalJSON()
	require.NoError(t, err)

	assert.JSONEq(t, `{
	  "allocations": [
	    {"warehouseId": 1, "take": 30},
	    {"warehouseId": 2, "take": 50.5},
	    {"warehouseId": 3, "take": 19.5},
	    {"warehouseId": 4, "take": 0}
	  ],
	  "allocationsState": {"remaining": 0}
	}`, string(result))
}
