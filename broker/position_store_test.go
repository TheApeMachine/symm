package broker

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

func TestPositionStoreSave(t *testing.T) {
	Convey("Given an open position stoploss", t, func() {
		store := newPositionStoreFixture(t)
		stoploss := newBrokerStoploss(t)
		stoploss.Update(stoploss.ArmAt)
		So(store.Save(stoploss), ShouldBeNil)

		Convey("It should restore and delete the exact live state", func() {
			restored, err := store.Load(t.Context(), stoploss.Symbol)
			So(err, ShouldBeNil)
			So(restored, ShouldNotBeNil)
			So(restored.Floor.Cmp(stoploss.Floor), ShouldEqual, 0)
			So(restored.Locked, ShouldEqual, stoploss.Locked)

			So(store.Delete(stoploss.Symbol), ShouldBeNil)
			restored, err = store.Load(t.Context(), stoploss.Symbol)
			So(err, ShouldBeNil)
			So(restored, ShouldBeNil)
		})
	})
}

func newPositionStoreFixture(testingTB testing.TB) *PositionStore {
	testingTB.Helper()
	store, err := NewPositionStore(
		filepath.Join(testingTB.TempDir(), "symm.sqlite"),
	)

	if err != nil {
		testingTB.Fatalf("position store: %v", err)
	}

	testingTB.Cleanup(func() {
		if err := store.Close(); err != nil {
			testingTB.Errorf("close position store: %v", err)
		}
	})

	return store
}

func newBrokerStoploss(testingTB testing.TB) *types.Stoploss {
	testingTB.Helper()
	forecast, err := types.NewResonanceForecast(
		[]float64{-0.01, 0.03},
		[]float64{1, 1},
		2,
		0.95,
	)

	if err != nil {
		testingTB.Fatalf("forecast: %v", err)
	}

	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := types.NewStoploss(
		context.Background(),
		"SIM/USD",
		decimal.NewFromFloat64(100.02),
		decimal.NewFromFloat64(100),
		forecast,
		decimal.NewFromFloat64(0.01),
		zeroRate,
		zeroRate,
	)

	if err != nil {
		testingTB.Fatalf("stoploss: %v", err)
	}

	return stoploss
}

func BenchmarkPositionStoreSave(b *testing.B) {
	store := newPositionStoreFixture(b)
	stoploss := newBrokerStoploss(b)
	b.ResetTimer()

	for b.Loop() {
		if err := store.Save(stoploss); err != nil {
			b.Fatalf("save stoploss: %v", err)
		}
	}
}
