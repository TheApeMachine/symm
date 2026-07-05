package leadlag

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestLagPayloadClassification(t *testing.T) {
	Convey("Given leadlag feature fixtures", t, func() {
		type lagCase struct {
			name     string
			features LagFeatures
			category logic.CategoryType
		}

		cases := []lagCase{
			{
				name: "inefficient lag",
				features: LagFeatures{
					Price:       100,
					MoveMoved:   true,
					LagOK:       true,
					LagBars:     8,
					LagCorr:     0.9,
					ContempOK:   true,
					ContempCorr: 0.1,
					SampleCount: 64,
				},
				category: logic.CategoryInefficientLag,
			},
			{
				name: "synchronized drift",
				features: LagFeatures{
					Price:       100,
					MoveMoved:   true,
					LagOK:       true,
					LagBars:     0,
					LagCorr:     0.1,
					ContempOK:   true,
					ContempCorr: 0.9,
					SampleCount: 64,
				},
				category: logic.CategorySynchronizedDrift,
			},
			{
				name: "decoupled move",
				features: LagFeatures{
					Price:       100,
					MoveMoved:   true,
					LagOK:       true,
					LagBars:     0,
					LagCorr:     0.01,
					ContempOK:   true,
					ContempCorr: 0.01,
					SampleCount: 64,
				},
				category: logic.CategoryDecoupledMove,
			},
			{
				name: "anchor stall",
				features: LagFeatures{
					Price:       100,
					MoveMoved:   false,
					LagOK:       true,
					LagBars:     0,
					LagCorr:     0.01,
					ContempOK:   true,
					ContempCorr: 0.01,
					SampleCount: 64,
					StallMargin: 0.9,
				},
				category: logic.CategoryAnchorStall,
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				ticker := NewTicker(NewSection())
				measurement, err := ticker.measurementFromFeatures(
					"ETH/EUR",
					time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
					testCase.features,
				)

				Convey("Then the intended category should dominate", func() {
					So(err, ShouldBeNil)
					So(measurement, ShouldNotBeNil)
					So(measurement.DominantCategory(), ShouldEqual, testCase.category)
				})
			})
		}
	})
}
