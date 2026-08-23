package tests

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	tes "github.com/theapemachine/symm/tests/types"
)

func TestFaultInjectorApply(t *testing.T) {
	Convey("Given an exact deterministic fault sequence", t, func() {
		injector := newFaultInjector(tes.FaultConfig{
			Seed: 9,
			Rules: []tes.FaultRule{
				{Channel: "ticker", Occurrence: 1, Action: tes.FaultReorder},
				{Channel: "ticker", Occurrence: 3, Action: tes.FaultDuplicate},
				{Channel: "ticker", Occurrence: 4, Action: tes.FaultDrop},
			},
		})

		first := injector.Apply("ticker", []byte(`{"sequence":1}`))
		second := injector.Apply("ticker", []byte(`{"sequence":2}`))
		third := injector.Apply("ticker", []byte(`{"sequence":3}`))
		fourth := injector.Apply("ticker", []byte(`{"sequence":4}`))

		Convey("It should reorder, duplicate, and drop only named occurrences", func() {
			So(first.frames, ShouldBeEmpty)
			So(second.frames, ShouldHaveLength, 2)
			So(string(second.frames[0]), ShouldEqual, `{"sequence":2}`)
			So(string(second.frames[1]), ShouldEqual, `{"sequence":1}`)
			So(third.frames, ShouldHaveLength, 2)
			So(fourth.frames, ShouldBeEmpty)

			report := injector.Report()
			So(report.Published, ShouldEqual, 4)
			So(report.Reordered, ShouldEqual, 1)
			So(report.Duplicated, ShouldEqual, 1)
			So(report.Dropped, ShouldEqual, 1)
			So(report.Frames, ShouldHaveLength, 4)
		})
	})
}

func TestFaultSequenceGap(t *testing.T) {
	Convey("Given a sequenced private event", t, func() {
		payload := sequenceGap([]byte(`{"channel":"balances","sequence":7}`), 3)
		wire := map[string]any{}
		err := json.Unmarshal(payload, &wire)

		Convey("It should expose the configured missing sequence range", func() {
			So(err, ShouldBeNil)
			So(wire["sequence"], ShouldEqual, float64(10))
		})
	})

	Convey("Given a channel without venue sequence semantics", t, func() {
		Convey("A sequence-gap fault should fail loudly", func() {
			So(func() {
				sequenceGap([]byte(`{"channel":"ticker"}`), 1)
			}, ShouldPanic)
		})
	})
}

func TestFaultInjectorLatency(t *testing.T) {
	Convey("Given two injectors with the same latency seed", t, func() {
		config := tes.FaultConfig{
			Seed: 17,
			ChannelLatency: map[string]tes.LatencyConfig{
				"ticker": {Base: 10 * time.Millisecond, Jitter: 5 * time.Millisecond},
			},
		}
		first := newFaultInjector(config).Apply("ticker", []byte(`{"channel":"ticker"}`))
		second := newFaultInjector(config).Apply("ticker", []byte(`{"channel":"ticker"}`))

		Convey("The sampled delay should be reproducible and bounded", func() {
			So(first.delay, ShouldEqual, second.delay)
			So(first.delay, ShouldBeGreaterThanOrEqualTo, 5*time.Millisecond)
			So(first.delay, ShouldBeLessThanOrEqualTo, 15*time.Millisecond)
		})
	})
}

func TestFaultInjectorStaleAndMalformed(t *testing.T) {
	Convey("Given stale and malformed frame rules", t, func() {
		injector := newFaultInjector(tes.FaultConfig{Rules: []tes.FaultRule{
			{Channel: "book", Occurrence: 2, Action: tes.FaultStale},
			{Channel: "book", Occurrence: 3, Action: tes.FaultMalformed},
		}})
		first := injector.Apply("book", []byte(`{"sequence":1}`))
		second := injector.Apply("book", []byte(`{"sequence":2}`))
		third := injector.Apply("book", []byte(`{"sequence":3}`))

		Convey("The stale payload should repeat and malformed bytes should remain exact", func() {
			So(string(second.frames[0]), ShouldEqual, string(first.frames[0]))
			So(string(third.frames[0]), ShouldEqual, "{")
			So(injector.Report().Stale, ShouldEqual, 1)
			So(injector.Report().Malformed, ShouldEqual, 1)
		})
	})
}

func BenchmarkFaultInjectorApply(b *testing.B) {
	injector := newFaultInjector(tes.FaultConfig{Seed: 19})
	payload := []byte(`{"channel":"ticker","data":[{"symbol":"BENCH/USD"}]}`)

	for b.Loop() {
		injector.Apply("ticker", payload)
		injector.mu.Lock()
		injector.report.Frames = injector.report.Frames[:0]
		injector.mu.Unlock()
	}
}
