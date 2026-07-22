package types

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestCaptureRecovery proves durable state rejects non-finite stop geometry
instead of silently replacing economic values with zero.
*/
func TestCaptureRecovery(t *testing.T) {
	Convey("Given an open holding with non-finite stop geometry", t, func() {
		qty := decimal.NewFromFloat64(1.5)
		weight := math.NaN()
		peak := math.Inf(1)
		open := map[string]Holding{
			"ONDO/USD": {
				Symbol: "ONDO/USD",
				Asset:  "ONDO",
				Qty:    qty,
				Status: OPEN,
				Stoploss: &Stoploss{
					Weight:     weight,
					PeakReturn: peak,
				},
			},
		}

		Convey("When CaptureRecovery builds the checkpoint", func() {
			recovery, err := CaptureRecovery(9, open, nil, nil)

			Convey("Then it should expose the invalid durable state", func() {
				So(recovery, ShouldBeNil)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "non-finite weight")
			})
		})
	})
}

func TestSaveLoadRecovery(t *testing.T) {
	Convey("Given a durable recovery directory", t, func() {
		dir := t.TempDir()
		qty, err := decimal.NewFromString("2.123456789123456789")
		So(err, ShouldBeNil)
		reservation, err := decimal.NewFromString("0.123456789123456789")
		So(err, ShouldBeNil)
		original := &Recovery{
			Tick: 3,
			Reservations: []ReservationWire{
				{ID: "exact", Amount: reservation},
			},
			Holdings: map[string]Holding{
				"UAI/USD": {
					Symbol: "UAI/USD",
					Asset:  "UAI",
					Qty:    qty,
					Status: OPEN,
				},
			},
		}

		Convey("When SaveRecovery and LoadRecovery round-trip", func() {
			So(SaveRecovery(dir, original), ShouldBeNil)

			loaded, err := LoadRecovery(dir)
			So(err, ShouldBeNil)
			So(loaded, ShouldNotBeNil)
			So(loaded.Tick, ShouldEqual, 3)
			So(loaded.Holdings, ShouldContainKey, "UAI/USD")
			So(loaded.Holdings["UAI/USD"].Asset, ShouldEqual, "UAI")
			So(loaded.Holdings["UAI/USD"].Qty.String(), ShouldEqual, qty.String())
			So(loaded.Reservations, ShouldHaveLength, 1)
			So(loaded.Reservations[0].Amount.String(), ShouldEqual, reservation.String())
		})

		Convey("When the file is missing", func() {
			loaded, err := LoadRecovery(filepath.Join(dir, "missing"))
			So(err, ShouldBeNil)
			So(loaded, ShouldBeNil)
		})

		Convey("When manually assembled recovery contains a non-finite stop", func() {
			original.Holdings["UAI/USD"] = Holding{
				Symbol: "UAI/USD",
				Qty:    qty,
				Status: OPEN,
				Stoploss: &Stoploss{
					Weight: math.Inf(1),
				},
			}
			err := SaveRecovery(dir, original)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "non-finite weight")
		})
	})
}

func TestHoldingEnrich(t *testing.T) {
	Convey("Given a wallet shell missing entry economics", t, func() {
		live := &Holding{
			Symbol: "ONDO/USD",
			Asset:  "ONDO",
			Qty:    decimal.NewFromFloat64(10),
			Status: OPEN,
		}
		entry := decimal.NewFromFloat64(1.25)
		recovered := Holding{
			Symbol:     "ONDO/USD",
			EntryPrice: entry,
			Stoploss: &Stoploss{
				Weight:        0.4,
				Action:        "hold",
				LockedFloor:   0.03,
				PeakReturn:    0.08,
				MarkReturn:    0.05,
				TrailDistance: 0.02,
				StopReturn:    0.03,
				FloorDistance: 0.02,
			},
		}
		recovered.Stoploss.NoteRetreat(0.9)

		payload, err := json.Marshal(Recovery{
			Holdings: map[string]Holding{"ONDO/USD": recovered},
		})
		So(err, ShouldBeNil)

		var roundtrip Recovery
		So(json.Unmarshal(payload, &roundtrip), ShouldBeNil)
		So(roundtrip.Holdings, ShouldContainKey, "ONDO/USD")

		live.Enrich(roundtrip.Holdings["ONDO/USD"])

		Convey("Then durable fields restore without resetting the trail", func() {
			So(live.EntryPrice, ShouldNotBeNil)
			So(live.EntryPrice.Float64(), ShouldEqual, 1.25)
			So(live.Stoploss, ShouldNotBeNil)
			So(live.Stoploss.Weight, ShouldEqual, 0.4)
			So(live.Stoploss.Armed(), ShouldBeTrue)
			So(live.Stoploss.LockedFloor, ShouldEqual, 0.03)
			So(live.Stoploss.PeakReturn, ShouldEqual, 0.08)
		})

		Convey("Then retreat survives JSON recovery", func() {
			adverse := live.Stoploss.Update(StopEvidence{
				Symbol: "ONDO/USD", Mark: 1.20, Entry: 1.25,
				Uncertainty: 0.02, ExpectedReturn: 0.05, Present: true,
			})

			So(adverse.Action, ShouldEqual, "hold")
			So(adverse.Reason, ShouldContainSubstring, "retreat")
		})
	})
}

func BenchmarkCaptureRecovery(b *testing.B) {
	qty := decimal.NewFromFloat64(1)
	open := map[string]Holding{
		"BTC/USD": {
			Symbol: "BTC/USD", Qty: qty, Status: OPEN,
			Stoploss: &Stoploss{Weight: 0.5},
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := CaptureRecovery(1, open, map[string]PendingOrderWire{
			"BTC/USD": {Symbol: "BTC/USD", Side: "sell", OrderID: "oid", Intent: "exit_pending"},
		}, nil)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestLoadRecoveryMalformed(t *testing.T) {
	Convey("Given a malformed recovery file", t, func() {
		dir := t.TempDir()
		So(os.WriteFile(filepath.Join(dir, RecoveryKey+".json"), []byte(`{`), 0o600), ShouldBeNil)

		loaded, err := LoadRecovery(dir)
		So(loaded, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})
}
