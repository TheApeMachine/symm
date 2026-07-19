package catalog_test

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
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

			theses, err := session.Play(entry.Frames())

			if err != nil {
				t.Fatalf("play: %v", err)
			}

			if err := entry.AssertMeasure(theses); err != nil {
				t.Fatal(err)
			}

			if entry.IsExitProof() {
				catalog.ProveExit(t, entry.Kind, catalog.Signals)
				return
			}

			needsStrategy := entry.Truth.DecideAction != "" ||
				entry.Truth.MustNotEnter ||
				entry.Truth.SizedEnter ||
				entry.Truth.WalletBound != catalog.WalletBoundNone

			if !needsStrategy {
				return
			}

			catalog.ProveStrategyOnSession(t, session, entry)
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

	frames := entry.Frames()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := session.Play(frames); err != nil {
			b.Fatal(err)
		}
	}
}
