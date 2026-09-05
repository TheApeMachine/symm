package advisor

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMetricPrediction(t *testing.T) {
	Convey("Given opposing moves for one adaptive metric", t, func() {
		prediction := NewMetricPrediction("pumpdump/notional_rate_velocity", INCREASE, DECREASE)

		Convey("it declares raw zero-threshold support and contradiction events", func() {
			So(prediction.Support.Label, ShouldEqual, "pumpdump/notional_rate_velocity")
			So(prediction.Support.Type, ShouldEqual, METRIC)
			So(prediction.Support.Move, ShouldEqual, INCREASE)
			So(prediction.Support.Value, ShouldEqual, 0.0)
			So(prediction.Support.Unit, ShouldEqual, RAW)
			So(prediction.Contradict.Label, ShouldEqual, prediction.Support.Label)
			So(prediction.Contradict.Move, ShouldEqual, DECREASE)
		})
	})
}

/*
declaredMetrics loads signal/metric_map.csv, the canonical declaration of every
metric a signal projects, as source -> metric.
*/
func declaredMetrics(t *testing.T) map[string]bool {
	t.Helper()

	handle, err := os.Open(filepath.Join("..", "..", "signal", "metric_map.csv"))

	if err != nil {
		t.Fatalf("open metric map: %v", err)
	}

	defer handle.Close()

	rows, err := csv.NewReader(handle).ReadAll()

	if err != nil {
		t.Fatalf("read metric map: %v", err)
	}

	declared := make(map[string]bool, len(rows))

	for _, row := range rows[1:] {
		if len(row) < 2 {
			continue
		}

		declared[row[0]+"/"+row[1]] = true
	}

	return declared
}

func advisorFeatures() map[string][]*Feature {
	return map[string][]*Feature{
		MomentumName:      NewMomentum().Features,
		AuctionName:       NewAuction().Features,
		ParticipationName: NewParticipation().Features,
		PullbackName:      NewPullback().Features,
		ProfitRunName:     NewProfitRun().Features,
		LiquidityName:     NewLiquidity().Features,
		BasisName:         NewBasis().Features,
	}
}

/*
TestNewFeature holds every Advisor's evidence contract to metrics that are actually declared.
*/
func TestNewFeature(t *testing.T) {
	Convey("Given the declared metric map", t, func() {
		declared := declaredMetrics(t)
		So(declared, ShouldNotBeEmpty)

		Convey("every Advisor evidence key names a declared metric", func() {
			for name, features := range advisorFeatures() {
				for _, feature := range features {
					for _, key := range feature.Keys {
						So(
							name+" requires "+key+" (declared: "+
								boolWord(declared[key])+")",
							ShouldEqual,
							name+" requires "+key+" (declared: true)",
						)
					}
				}
			}
		})

		Convey("every Advisor market clock names a declared metric", func() {
			for name, features := range advisorFeatures() {
				for _, feature := range features {
					So(
						name+" clock "+feature.Clock+" declared: "+
							boolWord(declared[feature.Clock]),
						ShouldEqual,
						name+" clock "+feature.Clock+" declared: true",
					)
				}
			}
		})

		Convey("every evidence key is source-qualified", func() {
			for _, features := range advisorFeatures() {
				for _, feature := range features {
					So(strings.Count(feature.Clock, "/"), ShouldEqual, 1)
				}
			}
		})
	})
}

func boolWord(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

