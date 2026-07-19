package catalog_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogIncompleteCutRefusesEnter proves CommitStrategy manages honesty: when
the Thesis cut is marked incomplete, sized enters are refused even with a seeded
fundable forecast (Session / production Plan path).
*/
func TestCatalogIncompleteCutRefusesEnter(t *testing.T) {
	Convey("Given a Session with a fundable opportunity forecast", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: catalog.Signals,
		})
		So(err, ShouldBeNil)

		entry := catalog.MustKind(t, catalog.KindFundedSlice)
		_, err = session.Play(entry.Frames())
		So(err, ShouldBeNil)
		So(session.SeedTakerFee(entry.Symbol, entry.FeePct), ShouldBeNil)
		So(session.SeedQuoteCapital(entry.Capital), ShouldBeNil)
		session.Desk.SetSlots(2, 2)

		thesis := types.NewThesis(nil, nil)
		tests.SeedOpportunityForecast(thesis, entry.Symbol, 0.12, 0.02)
		tests.SeedEarlyCognition(thesis, entry.Symbol)
		thesis.NoteIncomplete()

		Convey("When CommitStrategy runs on an incomplete cut", func() {
			So(session.CommitStrategy(thesis), ShouldBeNil)

			Convey("Then fresh enters are refused with measure_incomplete", func() {
				enter := false
				incomplete := false

				for _, decision := range thesis.Decisions {
					if decision.Symbol == entry.Symbol &&
						decision.Action == types.ActionEnter &&
						decision.ProposedQuantity != nil &&
						decision.ProposedQuantity.Sign() > 0 {
						enter = true
					}

					if decision.Cause == "measure_incomplete" {
						incomplete = true
					}
				}

				So(enter, ShouldBeFalse)
				So(incomplete, ShouldBeTrue)
			})
		})
	})
}
