package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestTape(t *testing.T) {
	Convey("Setup", t, func() {
		base := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
		window := system.Cfg.Risk.TapeWindow

		cases := []struct {
			name         string
			seed         func(*Tape)
			symbol       string
			at           time.Time
			wantRecent   float64
			wantPrior    float64
		}{
			{
				name: "quote dollars in recent versus prior window",
				seed: func(tape *Tape) {
					tape.Note("Q/USD", 40, base.Add(-window-time.Minute))
					tape.Note("Q/USD", 200, base.Add(-time.Minute))
				},
				symbol:     "Q/USD",
				at:         base,
				wantRecent: 200,
				wantPrior:  40,
			},
			{
				name: "minute buckets split at the window boundary",
				seed: func(tape *Tape) {
					tape.Note("Q/USD", 10, base.Add(-window+time.Minute))
					tape.Note("Q/USD", 7, base.Add(-window-time.Minute))
					tape.Note("Q/USD", 3, base.Add(-2*window+time.Minute))
					tape.Note("Q/USD", 99, base.Add(-2*window-time.Minute))
				},
				symbol:     "Q/USD",
				at:         base,
				wantRecent: 10,
				wantPrior:  10,
			},
			{
				name: "dust notes and missing symbols are zero",
				seed: func(tape *Tape) {
					tape.Note("Q/USD", 0, base)
					tape.Note("", 80, base)
					tape.Note("Q/USD", 80, time.Time{})
				},
				symbol:     "Q/USD",
				at:         base,
				wantRecent: 0,
				wantPrior:  0,
			},
			{
				name: "stale slots drop once the ring wraps",
				seed: func(tape *Tape) {
					tape.Note("Q/USD", 5, base.Add(-3*window))
					tape.Note("Q/USD", 8, base.Add(-time.Minute))
				},
				symbol:     "Q/USD",
				at:         base,
				wantRecent: 8,
				wantPrior:  0,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				tape := NewTape()
				tc.seed(tape)

				recent, prior := tape.Uptake(tc.symbol, tc.at)
				So(recent, ShouldEqual, tc.wantRecent)
				So(prior, ShouldEqual, tc.wantPrior)
			})
		}

		Convey("When the symbol was never noted", func() {
			tape := NewTape()
			recent, prior := tape.Uptake("MISSING/USD", base)
			So(recent, ShouldEqual, 0)
			So(prior, ShouldEqual, 0)
		})
	})
}
