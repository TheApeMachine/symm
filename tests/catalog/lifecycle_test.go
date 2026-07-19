package catalog_test

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogLifecycleProofs runs one full-stack proof per catalog opportunity
or trap: mock Conns → stack.Boot → Play (Crypto.Tick) → optional strategy
commit with seeded forecasts → staged known outcomes. Signals match production.
*/
func TestCatalogLifecycleProofs(t *testing.T) {
	for _, entry := range catalog.All() {
		entry := entry
		t.Run(string(entry.Kind)+"/"+entry.Name, func(t *testing.T) {
			session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
				Signals: catalog.Signals,
			})

			if err != nil {
				t.Fatalf("boot: %v", err)
			}

			theses, err := session.Play(catalog.Frames(entry))

			if err != nil {
				t.Fatalf("play: %v", err)
			}

			if err := catalog.AssertMeasure(theses, entry); err != nil {
				t.Fatal(err)
			}

			needsStrategy := entry.Truth.DecideAction != "" ||
				entry.Truth.MustNotEnter ||
				entry.Truth.SizedEnter ||
				entry.Truth.WalletBound != ""

			if !needsStrategy {
				return
			}

			if err := session.SeedTakerFee(entry.Symbol, entry.FeePct); err != nil {
				t.Fatal(err)
			}

			if err := session.SeedQuoteCapital(entry.Capital); err != nil {
				t.Fatal(err)
			}

			before, err := session.Balance.AvailableQuote()

			if err != nil {
				t.Fatal(err)
			}

			session.Desk.SetSlots(2, 2)
			thesis := types.NewThesis(nil, nil)

			if entry.Truth.DecideAction == types.ActionEnter ||
				entry.Truth.SizedEnter ||
				entry.Kind == catalog.KindUnfundableLot {
				tests.SeedOpportunityForecast(thesis, entry.Symbol, 0.12, 0.02)
				tests.SeedEarlyCognition(thesis, entry.Symbol)
			}

			if err := session.CommitStrategy(thesis); err != nil {
				t.Fatalf("CommitStrategy: %v", err)
			}

			if err := catalog.AssertDecide(thesis, entry); err != nil {
				t.Fatal(err)
			}

			after, err := session.Balance.AvailableQuote()

			if err != nil {
				t.Fatal(err)
			}

			if entry.Truth.WalletBound == "deploy" {
				if err := catalog.AssertSizedEnter(thesis, entry); err != nil {
					t.Fatal(err)
				}

				return
			}

			if err := catalog.AssertWallet(before, after, entry); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func BenchmarkCatalogPumpTick(b *testing.B) {
	entry := catalog.All()[0]
	session, err := tests.NewSession(context.Background(), b, tests.SessionOptions{
		Signals: catalog.Signals,
	})

	if err != nil {
		b.Fatal(err)
	}

	frames := catalog.Frames(entry)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := session.Play(frames); err != nil {
			b.Fatal(err)
		}
	}
}
