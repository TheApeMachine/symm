package ui

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestHubRetain proves the dashboard cache retains each supported top-level frame
exactly while ignoring analyzer frames, nested lookalikes, and malformed input.
*/
func TestHubRetain(t *testing.T) {
	proofs := []struct {
		name     string
		payload  string
		expected map[string]string
	}{
		{
			name:    "analyzer frame",
			payload: `{"manifold":[{"symbol":"SIM1/USD"}]}`,
		},
		{
			name:    "nested cache key",
			payload: `{"graphs":{"measurements":[]}}`,
		},
		{
			name:    "malformed cache frame",
			payload: `{"balances":`,
		},
		{
			name:    "single cache frame",
			payload: `{"balances":[{"asset":"USD","balance":1}]}`,
			expected: map[string]string{
				"balances": `{"balances":[{"asset":"USD","balance":1}]}`,
			},
		},
		{
			name:    "combined cache frame",
			payload: `{"balances":[{"asset":"USD","balance":1}],"holdings":[{"symbol":"SIM1/USD"}]}`,
			expected: map[string]string{
				"balances": `{"balances":[{"asset":"USD","balance":1}]}`,
				"holdings": `{"holdings":[{"symbol":"SIM1/USD"}]}`,
			},
		},
	}

	Convey("Given cacheable and non-cacheable dashboard frames", t, func() {
		for _, proof := range proofs {
			Convey(proof.name, func() {
				hub := &Hub{subscribers: &sync.Map{}}
				payload := []byte(proof.payload)
				hub.retain(payload)
				retained := 0

				if proof.name == "analyzer frame" {
					allocations := testing.AllocsPerRun(100, func() {
						hub.retain(payload)
					})
					So(allocations, ShouldEqual, 0)
				}

				hub.subscribers.Range(func(key, value any) bool {
					retained++
					So(string(value.([]byte)), ShouldEqual, proof.expected[key.(string)])
					return true
				})

				So(retained, ShouldEqual, len(proof.expected))
			})
		}
	})
}
