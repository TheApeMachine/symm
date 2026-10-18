package audit

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/system"
)

func TestVitalityStage(t *testing.T) {
	Convey("Given a VitalityStage instance", t, func() {
		ctx := context.Background()
		stage := NewVitalityStage(ctx)

		feed := func(s *VitalityStage, ms ...*data.Value) []*Violation {
			for _, m := range ms {
				s.Channel() <- m
			}
			close(s.Channel())
			So(s.Run(), ShouldBeNil)
			return s.Violations()
		}

		Convey("When the catalog signal changes on a symbol whose price moved", func() {
			stage.catalog = []catalogMetric{{Origin: "pumpdump", Name: "volume_rate"}}
			measurements := make([]*data.Value, 0, 8)

			for idx := range int64(8) {
				val := data.NewValue("pumpdump", "signal", "BTC/USD")
				val.Append([]float64{100 + float64(idx), float64(idx + 1)}, "best_price:bid", "volume_rate")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.active, ShouldResemble, []string{"BTC/USD"})
		})

		Convey("When a quiet symbol is flat and an active symbol moves", func() {
			stage.catalog = []catalogMetric{{Origin: "pumpdump", Name: "volume_rate"}}
			measurements := make([]*data.Value, 0, 16)

			for range int64(8) {
				quiet := data.NewValue("pumpdump", "signal", "QUIET/USD")
				quiet.Append([]float64{50, 0}, "best_price:bid", "volume_rate")
				measurements = append(measurements, quiet)
			}

			for idx := range int64(8) {
				active := data.NewValue("pumpdump", "signal", "BTC/USD")
				active.Append([]float64{100 + float64(idx), float64(idx + 1)}, "best_price:bid", "volume_rate")
				measurements = append(measurements, active)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.active, ShouldResemble, []string{"BTC/USD"})
		})

		Convey("When the signal stays at one non-zero value on the active symbol", func() {
			stage.catalog = []catalogMetric{{Origin: "sentiment", Name: "breadth"}}
			measurements := make([]*data.Value, 0, 8)

			for idx := range int64(8) {
				val := data.NewValue("sentiment", "signal", "CRO/USD")
				val.Append([]float64{1 + float64(idx)*0.01, -0.081}, "best_price:bid", "breadth")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(hasViolation(violations, "vitality.dead_metric"), ShouldBeTrue)
			So(stage.Report(), ShouldContainSubstring, "sentiment.breadth")
		})

		Convey("When the signal is zero on every sample from the active symbol", func() {
			stage.catalog = []catalogMetric{{Origin: "sentiment", Name: "historical_path_distance"}}
			measurements := make([]*data.Value, 0, 8)

			for idx := range int64(8) {
				val := data.NewValue("sentiment", "signal", "BTC/USD")
				val.Append([]float64{100 + float64(idx), 0}, "best_price:bid", "historical_path_distance")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(hasViolation(violations, "vitality.unmeasured"), ShouldBeTrue)
			So(hasViolation(violations, "vitality.dead_metric"), ShouldBeFalse)
		})

		Convey("When a signal is usually zero but does change", func() {
			stage.catalog = []catalogMetric{{Origin: "pumpdump", Name: "positive_midpoint_return"}}
			measurements := make([]*data.Value, 0, 8)

			for idx := range int64(8) {
				value := 0.0
				if idx == 7 {
					value = 0.02
				}
				val := data.NewValue("pumpdump", "signal", "BTC/USD")
				val.Append([]float64{100 + float64(idx), value}, "best_price:bid", "positive_midpoint_return")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
		})

		Convey("When the fee is present and does not change", func() {
			stage.catalog = []catalogMetric{{Origin: "liquidity", Name: "fee_rate", Static: true}}
			measurements := make([]*data.Value, 0, 4)

			for idx := range int64(4) {
				val := data.NewValue("liquidity", "signal", "BTC/USD")
				val.Append([]float64{100 + float64(idx), 0.002}, "best_price:bid", "fee_rate")
				measurements = append(measurements, val)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
		})

		Convey("When only the most active symbols are judged", func() {
			saved := system.Cfg.Audit.ActiveSymbols
			system.Cfg.Audit.ActiveSymbols = 1
			defer func() { system.Cfg.Audit.ActiveSymbols = saved }()

			stage.catalog = []catalogMetric{{Origin: "pumpdump", Name: "volume_rate"}}
			measurements := make([]*data.Value, 0, 8)

			for idx := range int64(4) {
				big := data.NewValue("pumpdump", "signal", "BIG/USD")
				big.Append([]float64{100 + float64(idx)*50, float64(idx + 1)}, "best_price:bid", "volume_rate")
				measurements = append(measurements, big)

				small := data.NewValue("pumpdump", "signal", "SMALL/USD")
				small.Append([]float64{100 + float64(idx)*0.01, 0}, "best_price:bid", "volume_rate")
				measurements = append(measurements, small)
			}

			violations := feed(stage, measurements...)
			So(violations, ShouldBeEmpty)
			So(stage.active, ShouldResemble, []string{"BIG/USD"})
		})

		Convey("When auditing nil measurements", func() {
			nilViolations := feed(stage, nil)
			So(hasViolation(nilViolations, "measurement"), ShouldBeTrue)
			So(hasInsufficient(nilViolations), ShouldBeTrue)
		})
	})
}

func TestSignalCatalogMatchesDocument(t *testing.T) {
	Convey("Given SIGNALS.md", t, func() {
		_, file, _, ok := runtime.Caller(0)
		So(ok, ShouldBeTrue)

		doc, err := os.Open(filepath.Join(filepath.Dir(file), "..", "..", "SIGNALS.md"))
		So(err, ShouldBeNil)
		defer doc.Close()

		documented := make(map[string]bool)
		scanner := bufio.NewScanner(doc)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "- [x] ") {
				continue
			}

			name := strings.TrimPrefix(line, "- [x] ")
			if name == "strategy.grammar" {
				continue
			}

			documented[name] = true
		}

		So(scanner.Err(), ShouldBeNil)

		catalog := make(map[string]bool)
		for _, metric := range signalCatalog() {
			catalog[metric.key()] = true
		}

		var missing, extra []string

		for name := range documented {
			if !catalog[name] {
				missing = append(missing, name)
			}
		}

		for name := range catalog {
			if !documented[name] {
				extra = append(extra, name)
			}
		}

		So(missing, ShouldBeEmpty)
		So(extra, ShouldBeEmpty)
	})
}
