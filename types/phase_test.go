package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhaseOutcome(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a PhaseOutcome is constructed", func() {
			out := PhaseOutcome{
				Direction: "up",
				Return:    0.0125,
				Horizon:   4,
			}

			Convey("fields retain the ground-truth label", func() {
				So(out.Direction, ShouldEqual, "up")
				So(out.Return, ShouldEqual, 0.0125)
				So(out.Horizon, ShouldEqual, 4)
			})
		})
	})
}

func TestPhaseResponse(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a PhaseResponse is constructed", func() {
			resp := PhaseResponse{
				Angle:      1.5707963267948966,
				Similarity: 0.82,
				ObservedAt: "2024-01-02T03:04:05Z",
				Outcome: PhaseOutcome{
					Direction: "down",
					Return:    -0.05,
					Horizon:   2,
				},
			}

			Convey("angle, similarity, and owned outcome are exact", func() {
				So(resp.Angle, ShouldEqual, 1.5707963267948966)
				So(resp.Similarity, ShouldEqual, 0.82)
				So(resp.ObservedAt, ShouldEqual, "2024-01-02T03:04:05Z")
				So(resp.Outcome.Direction, ShouldEqual, "down")
				So(resp.Outcome.Return, ShouldEqual, -0.05)
				So(resp.Outcome.Horizon, ShouldEqual, 2)
			})
		})
	})
}

func BenchmarkPhaseOutcome(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		out := PhaseOutcome{Direction: "up", Return: 0.0125, Horizon: 4}
		_ = out.Horizon
	}
}

func BenchmarkPhaseResponse(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		resp := PhaseResponse{
			Angle: 1.5707963267948966, Similarity: 0.82, ObservedAt: "2024-01-02T03:04:05Z",
			Outcome: PhaseOutcome{Direction: "down", Return: -0.05, Horizon: 2},
		}
		_ = resp.Similarity
	}
}
