package data

import "testing"

func TestLookupExactLayerKey(t *testing.T) {
	out := NewValue("training", "step", "BTC/USD")
	out.Append("path-only", "policyPath")
	out.Append(map[string]string{"match": "ok"}, "policy")

	var policy map[string]string
	out.Lookup(&policy, "policy")

	if policy["match"] != "ok" {
		t.Fatalf("policy = %#v, want match ok", policy)
	}

	if out.Err != nil {
		t.Fatalf("unexpected lookup error: %v", out.Err)
	}
}

func TestLookupNestedLayerKey(t *testing.T) {
	out := NewValue("workspace", "tick", "aggregate")
	out.Append([]string{"trade-payload"}, "spot:trade", "trade")

	var trades []string
	out.Lookup(&trades, "spot:trade")

	if len(trades) != 1 || trades[0] != "trade-payload" {
		t.Fatalf("trades = %#v", trades)
	}
}

func TestLookupDoesNotAliasSiblingKey(t *testing.T) {
	out := NewValue("training", "step", "BTC/USD")
	out.Append("wrong", "policyPath")

	var policy map[string]string
	out.Lookup(&policy, "policy")

	if policy != nil {
		t.Fatalf("policy = %#v, want absent", policy)
	}
}
