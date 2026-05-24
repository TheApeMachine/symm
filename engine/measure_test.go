package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestMeasureSymbolsSerial(t *testing.T) {
	Convey("Given a serial symbol scanner", t, func() {
		ctx := context.Background()
		now := time.Now().UTC()
		scanned := make([]string, 0, 3)

		for measurement := range MeasureSymbols(
			ctx,
			SymbolScanner{
				Source:  "test",
				Symbols: []string{"A/EUR", "B/EUR", "C/EUR"},
				Pairs: map[string]asset.Pair{
					"A/EUR": {Wsname: "A/EUR"},
					"B/EUR": {Wsname: "B/EUR"},
					"C/EUR": {Wsname: "C/EUR"},
				},
			},
			now,
			func(symbol string, _ Snapshot) (Measurement, bool, error) {
				scanned = append(scanned, symbol)

				return Measurement{Confidence: 0.5}, true, nil
			},
		) {
			_ = measurement
		}

		Convey("It should evaluate every symbol in order", func() {
			So(scanned, ShouldResemble, []string{"A/EUR", "B/EUR", "C/EUR"})
		})
	})
}

func TestMeasureSymbolsParallel(t *testing.T) {
	Convey("Given a qpool-backed symbol scanner", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())
		defer pool.Close()

		now := time.Now().UTC()
		var concurrent int32
		var peakConcurrent int32
		var scannedMu sync.Mutex
		scanned := make([]string, 0, 4)

		for measurement := range MeasureSymbols(
			ctx,
			SymbolScanner{
				Source:  "test",
				Symbols: []string{"A/EUR", "B/EUR", "C/EUR", "D/EUR"},
				Pairs: map[string]asset.Pair{
					"A/EUR": {Wsname: "A/EUR"},
					"B/EUR": {Wsname: "B/EUR"},
					"C/EUR": {Wsname: "C/EUR"},
					"D/EUR": {Wsname: "D/EUR"},
				},
				Pool: pool,
			},
			now,
			func(symbol string, _ Snapshot) (Measurement, bool, error) {
				active := atomic.AddInt32(&concurrent, 1)
				defer atomic.AddInt32(&concurrent, -1)

				for {
					currentPeak := atomic.LoadInt32(&peakConcurrent)

					if active <= currentPeak || atomic.CompareAndSwapInt32(&peakConcurrent, currentPeak, active) {
						break
					}
				}

				time.Sleep(5 * time.Millisecond)
				scannedMu.Lock()
				scanned = append(scanned, symbol)
				scannedMu.Unlock()

				return Measurement{Confidence: 0.5}, true, nil
			},
		) {
			_ = measurement
		}

		Convey("It should evaluate every symbol concurrently", func() {
			So(len(scanned), ShouldEqual, 4)
			So(peakConcurrent, ShouldBeGreaterThan, 1)
		})
	})
}

func TestRunSymbolJobs(t *testing.T) {
	Convey("Given parallel symbol jobs", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())
		defer pool.Close()

		seen := make([]string, 0, 2)

		err := RunSymbolJobs(ctx, pool, []string{"A/EUR", "B/EUR"}, func(symbol string) error {
			seen = append(seen, symbol)

			return nil
		})

		Convey("It should run every symbol job", func() {
			So(err, ShouldBeNil)
			So(len(seen), ShouldEqual, 2)
		})
	})
}
