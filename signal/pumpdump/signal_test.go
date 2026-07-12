package pumpdump

import (
	"context"
	"iter"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer func() { _ = signal.Close() }()

		Convey("When ingest roles are requested", func() {
			roles := signal.IngestRoles()

			Convey("Then it should consume only ticker data", func() {
				So(roles, ShouldResemble, []string{"ticker"})
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		signal := NewSignal[any](context.Background())
		defer func() { _ = signal.Close() }()

		Convey("When a mid-stream volume+price pump is injected", func() {
			calm := replay(t, signal, shapedTickerRows(nil))

			pumped := NewSignal[any](context.Background())
			defer func() { _ = pumped.Close() }()

			spiked := replay(t, pumped, shapedTickerRows(
				func(frames iter.Seq[tests.Frame]) iter.Seq[tests.Frame] {
					return tests.Spike(frames, 16, 1.25, 8)
				},
			))

			Convey("Then the signal's volume lift exceeds the calm baseline", func() {
				So(len(spiked), ShouldBeGreaterThan, 0)
				So(spiked[len(spiked)-1].Source, ShouldEqual, types.SourcePumpDump)
				So(peakMetric(spiked, "rvol"), ShouldBeGreaterThan, peakMetric(calm, "rvol"))
			})
		})

		Convey("When a non-ticker row is received", func() {
			measurements, err := signal.Measure(struct{}{}, nil)

			Convey("Then it should leave the row to its owning signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	inputs := tickerInputs()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal[any](context.Background())
		_ = replay(b, signal, inputs)
		_ = signal.Close()
	}
}

func replay(
	t testing.TB,
	signal *Signal[any],
	inputs []any,
) []*types.Measurement {
	t.Helper()

	measurements := make([]*types.Measurement, 0)
	for _, input := range inputs {
		out, err := signal.Measure(input, nil)
		if err != nil {
			t.Fatal(err)
		}

		measurements = append(measurements, out...)
	}

	return measurements
}

func shapedTickerRows(shape func(iter.Seq[tests.Frame]) iter.Seq[tests.Frame]) []any {
	fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)
	frames := fixture.Frames()

	if shape != nil {
		frames = shape(frames)
	}

	inputs := make([]any, 0, 32)

	for frame := range frames {
		ticker := kraken.Ticker{}

		if err := sonic.Unmarshal(frame.Payload, &ticker); err != nil {
			panic(err)
		}

		for _, row := range ticker.Data {
			inputs = append(inputs, row)
		}
	}

	return inputs
}

func peakMetric(measurements []*types.Measurement, key string) float64 {
	peak := 0.0

	for _, measurement := range measurements {
		if value := measurement.Metrics[key]; value > peak {
			peak = value
		}
	}

	return peak
}

func tickerInputs() []any {
	fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)
	inputs := make([]any, 0, 32)

	for payload := range fixture.Generate() {
		frame := kraken.Ticker{}

		if err := sonic.Unmarshal(payload, &frame); err != nil {
			panic(err)
		}

		for _, row := range frame.Data {
			inputs = append(inputs, row)
		}
	}

	return inputs
}
