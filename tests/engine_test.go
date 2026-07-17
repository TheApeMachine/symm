package tests

import (
	"iter"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEngineRun(t *testing.T) {
	Convey("Given a ticker update payload", t, func() {
		base := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"MATIC/USD","bid":0.10,"ask":0.11,"last":0.105,"volume":100,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When the engine drifts price forward", func() {
			engine := NewEngine(3).Drift(0.01).VolumeAdd(5).Interval(0)
			last := 0.0
			count := 0

			for payload := range engine.Run(base) {
				row := firstRow(t, payload)

				So(row["last"].(float64), ShouldBeGreaterThan, last)
				last = row["last"].(float64)
				count++
			}

			Convey("Then every step should trend upward", func() {
				So(count, ShouldEqual, 3)
			})
		})

		Convey("When jitter is enabled with a fixed seed", func() {
			first := firstRow(t, firstPayload(NewEngine(4).Drift(0.001).Jitter(0.02).Seed(7).Run(base)))
			second := firstRow(t, firstPayload(NewEngine(4).Drift(0.001).Jitter(0.02).Seed(7).Run(base)))

			Convey("Then the sequence should be repeatable", func() {
				So(first["last"], ShouldEqual, second["last"])
			})
		})
	})
}

func TestReplay(t *testing.T) {
	Convey("Given channel handlers", t, func() {
		seen := map[string]int{}
		handlers := Handlers{
			"ticker": func(payload []byte) {
				seen["ticker"]++
			},
			"trade": func(payload []byte) {
				seen["trade"]++
			},
		}

		frames := func(yield func(Frame) bool) {
			if !yield(Frame{Channel: "ticker", Type: "update", Payload: []byte(`{"channel":"ticker"}`)}) {
				return
			}

			if !yield(Frame{Channel: "trade", Type: "update", Payload: []byte(`{"channel":"trade"}`)}) {
				return
			}

			yield(Frame{Channel: "book", Type: "update", Payload: []byte(`{"channel":"book"}`)})
		}

		Convey("When frames are replayed", func() {
			Replay(handlers, iter.Seq[Frame](frames))

			Convey("Then only registered channels should fire", func() {
				So(seen["ticker"], ShouldEqual, 1)
				So(seen["trade"], ShouldEqual, 1)
				So(seen["book"], ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkEngineRun(b *testing.B) {
	base := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"MATIC/USD","bid":0.10,"ask":0.11,"last":0.105,"volume":100,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)
	engine := NewEngine(32).Drift(0.001).Jitter(0.005).VolumeAdd(10)

	b.ReportAllocs()

	for b.Loop() {
		for range engine.Run(base) {
		}
	}
}

func firstPayload(sequence iter.Seq[[]byte]) []byte {
	for payload := range sequence {
		return payload
	}

	return nil
}

func firstRow(t testing.TB, payload []byte) map[string]any {
	t.Helper()

	var frame map[string]any

	if err := sonic.Unmarshal(payload, &frame); err != nil {
		t.Fatal(err)
	}

	data := frame["data"].([]any)

	return data[0].(map[string]any)
}
