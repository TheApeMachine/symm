package broker

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
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

func TestPositionStoreSaveThesis(t *testing.T) {
	Convey("Given two completed thesis decision rounds", t, func() {
		store := newPositionStoreFixture(t)
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Tick = 1
		So(store.SaveThesis(thesis), ShouldBeNil)
		thesis.Tick = 2
		So(store.SaveThesis(thesis), ShouldBeNil)

		rows, err := store.database.Query(`
SELECT state FROM thesis_checkpoints ORDER BY id`)
		So(err, ShouldBeNil)
		defer rows.Close()
		ticks := make([]float64, 0)

		for rows.Next() {
			var state []byte
			So(rows.Scan(&state), ShouldBeNil)
			var checkpoint map[string]any
			So(json.Unmarshal(state, &checkpoint), ShouldBeNil)
			ticks = append(ticks, checkpoint["tick"].(float64))
		}

		Convey("It should append rather than overwrite the prior checkpoint", func() {
			So(rows.Err(), ShouldBeNil)
			So(ticks, ShouldResemble, []float64{1, 2})
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
	forecast := brokerForecast(0.03)

	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := types.NewStoploss(
		context.Background(),
		"SIM/USD",
		decimal.NewFromFloat64(100.02),
		decimal.NewFromFloat64(100),
		forecast,
		nil,
		decimal.NewFromFloat64(0.01),
		zeroRate,
		zeroRate,
	)

	if err != nil {
		testingTB.Fatalf("stoploss: %v", err)
	}

	return stoploss
}

func TestPositionStoreCheckpointRetention(t *testing.T) {
	Convey("Given more completed rounds than the retention window", t, func() {
		store := newPositionStoreFixture(t)
		thesis := types.NewThesis(t.Context(), nil)

		for round := range checkpointRetention + 8 {
			thesis.Tick = int64(round)
			So(store.SaveThesis(thesis), ShouldBeNil)
		}

		var retained int
		So(store.database.QueryRow(
			"SELECT COUNT(*) FROM thesis_checkpoints",
		).Scan(&retained), ShouldBeNil)

		Convey("It should keep only the newest checkpoints", func() {
			So(retained, ShouldEqual, checkpointRetention)

			var firstTick int64
			So(store.database.QueryRow(
				"SELECT json_extract(state, '$.tick') FROM thesis_checkpoints ORDER BY id LIMIT 1",
			).Scan(&firstTick), ShouldBeNil)
			So(firstTick, ShouldEqual, 8)
		})
	})
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
