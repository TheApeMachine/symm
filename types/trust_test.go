package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeObservationTrust(t *testing.T) {
	now := time.Unix(1000, 0).UTC()

	Convey("Given a fresh, mature, unambiguous, skilled observation", t, func() {
		node := &Node{
			Confidence: 0.9,
			Maturity:   1,
			At:         now,
			Kind:       KindMeasurement,
			Source:     "hawkes",
			Metadata: map[string]any{
				"task_skill": 2.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be trusted near fully", func() {
			So(trust, ShouldAlmostEqual, 0.9, 1e-9)
		})
	})

	Convey("Given a stale touch-level order flow observation", t, func() {
		node := &Node{
			Confidence: 0.9,
			Maturity:   1,
			At:         now.Add(-3 * time.Second),
			Kind:       KindMeasurement,
			Source:     "hawkes",
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be heavily attenuated by temporal freshness", func() {
			So(trust, ShouldBeLessThan, 0.02)
			So(trust, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given an immature observation below its maturity floor", t, func() {
		node := &Node{
			Confidence: 0.9,
			Maturity:   0.05,
			At:         now,
			Kind:       KindMeasurement,
			Source:     "cvd",
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be attenuated by maturity", func() {
			So(trust, ShouldAlmostEqual, 0.045, 1e-9)
		})
	})

	Convey("Given an ambiguous observation with separation zero", t, func() {
		node := &Node{
			Confidence: 0.9,
			Maturity:   1,
			At:         now,
			Kind:       KindMeasurement,
			Source:     "liquidity",
			Metadata: map[string]any{
				"hypothesis_separation": 0.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be fully attenuated", func() {
			So(trust, ShouldEqual, 0)
		})
	})

	Convey("Given an unskilled predictive coder", t, func() {
		node := &Node{
			Confidence: 0.9,
			Maturity:   1,
			At:         now,
			Kind:       KindResonance,
			Metadata: map[string]any{
				"task_skill": 0.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be fully attenuated", func() {
			So(trust, ShouldEqual, 0)
		})
	})

	Convey("Given a nil or zero-confidence node", t, func() {
		So(computeObservationTrust(nil, now), ShouldEqual, 0)
		So(computeObservationTrust(&Node{}, now), ShouldEqual, 0)
	})

	Convey("Trust must remain in the closed unit interval", t, func() {
		for maturity := 0.0; maturity <= 2; maturity += 0.25 {
			node := &Node{
				Confidence: 1.5,
				Maturity:   maturity,
				At:         now,
				Kind:       KindMeasurement,
				Source:     "sentiment",
			}

			trust := computeObservationTrust(node, now)
			So(trust, ShouldBeGreaterThanOrEqualTo, 0)
			So(trust, ShouldBeLessThanOrEqualTo, 1)
			So(math.IsNaN(trust), ShouldBeFalse)
		}
	})
}
