package driver

import (
	"testing"

	"github.com/theapemachine/symm/audit"
)

func TestDecodeDecisionEventCompatibility(t *testing.T) {
	payload := []byte(`{"action":"nothing","symbol":"DENT/USD","thesisScore":0.7}`)
	batchPayload := []byte(`[{"action":"nothing","symbol":"DENT/USD","thesisScore":0.7}]`)

	for _, testCase := range []struct {
		kind    string
		payload []byte
	}{
		{"types.Decision", payload},
		{"*types.Decision", payload},
		{"[]types.Decision", batchPayload},
		{"[]*types.Decision", batchPayload},
		{audit.DecisionBatchEvent, batchPayload},
	} {
		decisions, recognized, err := decodeDecisionEvent(testCase.kind, testCase.payload)
		if err != nil {
			t.Fatalf("decode %s: %v", testCase.kind, err)
		}
		if !recognized || len(decisions) != 1 || decisions[0].Symbol != "DENT/USD" {
			t.Fatalf("decode %s = %#v, recognized=%v", testCase.kind, decisions, recognized)
		}
	}

	if _, recognized, err := decodeDecisionEvent("other", payload); err != nil || recognized {
		t.Fatalf("unrelated event recognized=%v err=%v", recognized, err)
	}
}
