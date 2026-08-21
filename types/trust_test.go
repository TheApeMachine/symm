package types

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeObservationTrust(t *testing.T) {
	now := time.Unix(1000, 0).UTC()

	Convey("Given a fresh, mature, unambiguous measurement", t, func() {
		node := &Node{
			Maturity: 1,
			At:       now,
			Horizon:  time.Second,
			Kind:     KindMeasurement,
			Source:   "hawkes",
			Metadata: map[string]any{
				"hypothesis_separation": 1.0,
				"task_skill":            2.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be trusted fully", func() {
			So(trust, ShouldAlmostEqual, 1.0, 1e-9)
		})
	})

	Convey("Given a stale measurement whose own horizon has elapsed", t, func() {
		node := &Node{
			Maturity: 1,
			At:       now.Add(-time.Second),
			Horizon:  time.Second,
			Kind:     KindMeasurement,
			Source:   "hawkes",
			Metadata: map[string]any{
				"hypothesis_separation": 1.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should decay by exp(-age/horizon)", func() {
			So(trust, ShouldAlmostEqual, math.Exp(-1), 1e-9)
		})
	})

	Convey("Given an immature measurement", t, func() {
		node := &Node{
			Maturity: 0.05,
			At:       now,
			Kind:     KindMeasurement,
			Source:   "cvd",
			Metadata: map[string]any{
				"hypothesis_separation": 1.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be attenuated by maturity", func() {
			So(trust, ShouldAlmostEqual, 0.05, 1e-9)
		})
	})

	Convey("Given an ambiguous measurement with separation zero", t, func() {
		node := &Node{
			Maturity: 1,
			At:       now,
			Kind:     KindMeasurement,
			Source:   "liquidity",
			Metadata: map[string]any{
				"hypothesis_separation": 0.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be fully attenuated", func() {
			So(trust, ShouldEqual, 0)
		})
	})

	Convey("Given a measurement that never stamped separation", t, func() {
		node := &Node{
			Maturity: 1,
			At:       now,
			Kind:     KindMeasurement,
			Source:   "cvd",
		}

		Convey("It should carry no mass", func() {
			So(computeObservationTrust(node, now), ShouldEqual, 0)
		})
	})

	Convey("Given an unskilled predictive coder", t, func() {
		node := &Node{
			Maturity: 1,
			At:       now,
			Kind:     KindResonance,
			Metadata: map[string]any{
				"task_skill": 0.0,
			},
		}

		trust := computeObservationTrust(node, now)

		Convey("It should be fully attenuated", func() {
			So(trust, ShouldEqual, 0)
		})
	})

	Convey("Given a nil node", t, func() {
		So(computeObservationTrust(nil, now), ShouldEqual, 0)
		So(computeObservationTrust(&Node{}, now), ShouldEqual, 0)
	})

	Convey("Trust must remain in the closed unit interval", t, func() {
		for maturity := 0.0; maturity <= 2; maturity += 0.25 {
			node := &Node{
				Maturity: maturity,
				At:       now,
				Kind:     KindMeasurement,
				Source:   "sentiment",
				Metadata: map[string]any{
					"hypothesis_separation": 1.0,
				},
			}

			trust := computeObservationTrust(node, now)
			So(trust, ShouldBeGreaterThanOrEqualTo, 0)
			So(trust, ShouldBeLessThanOrEqualTo, 1)
			So(math.IsNaN(trust), ShouldBeFalse)
		}
	})
}
