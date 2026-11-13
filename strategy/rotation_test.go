package strategy

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

func TestNewRotation(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewRotation is called", func() {
			rotation := NewRotation(context.Background())

			Convey("Rotation starts with an empty ledger", func() {
				So(rotation, ShouldNotBeNil)
				So(rotation.Score("PIPE/USD"), ShouldEqual, 0)
			})
		})
	})
}

func TestRotationNoteSettled(t *testing.T) {
	Convey("Setup", t, func() {
		rotation := NewRotation(context.Background())
		win := decimal.NewFromFloat(1.0)
		loss := decimal.NewFromFloat(-0.5)

		Convey("When profitable closes are recorded", func() {
			rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})
			rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})

			Convey("Close and win counts rise", func() {
				rotation.mu.RLock()
				stat := rotation.stats["PIPE/USD"]
				rotation.mu.RUnlock()
				So(stat.Closes, ShouldEqual, 2)
				So(stat.Wins, ShouldEqual, 2)
			})
		})

		Convey("When a loss close is recorded", func() {
			rotation.NoteSettled(&broker.Position{Symbol: "LOSE/USD", PnLPct: &loss})

			Convey("Closes increment without a win", func() {
				rotation.mu.RLock()
				stat := rotation.stats["LOSE/USD"]
				rotation.mu.RUnlock()
				So(stat.Closes, ShouldEqual, 1)
				So(stat.Wins, ShouldEqual, 0)
			})
		})

		Convey("When position is nil", func() {
			Convey("NoteSettled is a no-op", func() {
				So(func() { rotation.NoteSettled(nil) }, ShouldNotPanic)
			})
		})
	})
}

func TestRotationScore(t *testing.T) {
	Convey("Setup", t, func() {
		rotation := NewRotation(context.Background())
		win := decimal.NewFromFloat(1.0)
		boost := system.Cfg.Selection.RotationCloseBoost

		Convey("When a symbol has two winning closes", func() {
			rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})
			rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})

			want := boost * 1.0 * 2

			Convey("Score is boost × win rate × capped closes", func() {
				So(rotation.Score("PIPE/USD"), ShouldEqual, want)
				So(rotation.Score("NEW/USD"), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkNewRotation(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		rotation := NewRotation(ctx)
		_ = rotation.Score("PIPE/USD")
	}
}

func BenchmarkRotationNoteSettled(b *testing.B) {
	rotation := NewRotation(context.Background())
	win := decimal.NewFromFloat(1.0)
	pos := &broker.Position{Symbol: "PIPE/USD", PnLPct: &win}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		rotation.NoteSettled(pos)
	}
}

func BenchmarkRotationScore(b *testing.B) {
	rotation := NewRotation(context.Background())
	win := decimal.NewFromFloat(1.0)
	rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})
	rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = rotation.Score("PIPE/USD")
	}
}
