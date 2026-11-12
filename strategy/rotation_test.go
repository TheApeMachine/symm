package strategy

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
)

func TestRotationScore(t *testing.T) {
	Convey("Rotation score rises with profitable closes", t, func() {
		rotation := NewRotation(context.Background())
		win := decimal.NewFromFloat(1.0)

		rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})
		rotation.NoteSettled(&broker.Position{Symbol: "PIPE/USD", PnLPct: &win})

		So(rotation.Score("PIPE/USD"), ShouldBeGreaterThan, 0)
		So(rotation.Score("NEW/USD"), ShouldEqual, 0)
	})
}
