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

func TestCaptureRecoveryNaNSafe(t *testing.T) {
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
					Skill: Skill{Weight: weight},
					Trail: Trail{PeakReturn: peak},
				},
			},
		}

		Convey("When CaptureRecovery builds the checkpoint", func() {
			recovery := CaptureRecovery(9, open, nil, nil)

			Convey("Then encoding/json accepts the payload", func() {
				So(recovery, ShouldNotBeNil)
				So(recovery.Holdings, ShouldContainKey, "ONDO/USD")
				So(math.IsNaN(recovery.Holdings["ONDO/USD"].Stoploss.Weight), ShouldBeFalse)
				So(math.IsInf(recovery.Holdings["ONDO/USD"].Stoploss.PeakReturn, 0), ShouldBeFalse)

				payload, err := json.Marshal(recovery)
				So(err, ShouldBeNil)
				So(len(payload), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSaveLoadRecovery(t *testing.T) {
	Convey("Given a durable recovery directory", t, func() {
		dir := t.TempDir()
		qty := decimal.NewFromFloat64(2)
		original := &Recovery{
			Tick: 3,
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
		})

		Convey("When the file is missing", func() {
			loaded, err := LoadRecovery(filepath.Join(dir, "missing"))
			So(err, ShouldBeNil)
			So(loaded, ShouldBeNil)
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
			Stoploss:   &Stoploss{Skill: Skill{Weight: 0.4}, Action: "hold"},
		}

		live.Enrich(recovered)

		Convey("Then durable fields are copied once", func() {
			So(live.EntryPrice, ShouldNotBeNil)
			So(live.EntryPrice.Float64(), ShouldEqual, 1.25)
			So(live.Stoploss, ShouldNotBeNil)
			So(live.Stoploss.Weight, ShouldEqual, 0.4)
		})
	})
}

func BenchmarkCaptureRecovery(b *testing.B) {
	qty := decimal.NewFromFloat64(1)
	open := map[string]Holding{
		"BTC/USD": {
			Symbol: "BTC/USD", Qty: qty, Status: OPEN,
			Stoploss: &Stoploss{Skill: Skill{Weight: 0.5}},
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = CaptureRecovery(1, open, map[string]string{"BTC/USD": "oid"}, nil)
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
