package data

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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

func TestLookupPrefixConcurrentAppend(t *testing.T) {
	Convey("LookupPrefix while Append runs on other goroutines", t, func() {
		value := NewValue("workspace", "tick", "aggregate")

		var group sync.WaitGroup
		stop := make(chan struct{})

		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-stop:
					return
				default:
					value.Append(1.0, "cvd", "BTC", "USD", "midpoint")
				}
			}
		}()

		for range 500 {
			entries := value.LookupPrefix("cvd")
			for _, entry := range entries {
				So(entry.Key, ShouldNotBeBlank)
			}
		}

		close(stop)
		group.Wait()
	})
}
