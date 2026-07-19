package catalog_test

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/catalog"
	"github.com/theapemachine/symm/types"
)

/*
TestCatalogLogicComposesGraphsFromTape proves Analyzer.Update ran on the real
Crypto.Tick path: after Play, the durable LastThesis carries composed evidence
graphs (CutSnapshot omits Graphs by design).
*/
func TestCatalogLogicComposesGraphsFromTape(t *testing.T) {
	entry := catalog.MustKind(t, catalog.KindPump)

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

	thesis := session.Crypto.LastThesis()

	if thesis == nil || thesis.Graphs == nil {
		t.Fatal("catalog logic: want durable LastThesis with Graphs map")
	}

	value, ok := thesis.Graphs.Load(entry.Symbol)

	if !ok || value == nil {
		t.Fatalf("catalog logic: want evidence graph for %s after pump tape", entry.Symbol)
	}

	evidenceGraph, ok := value.(*types.Graph)

	if !ok || len(evidenceGraph.Edges()) == 0 {
		t.Fatalf("catalog logic: want composed edges on %s graph", entry.Symbol)
	}

	if types.ObservationCount(thesis.Measurements) == 0 {
		t.Fatal("catalog logic: Analyzer path requires measurements on thesis")
	}
}

/*
TestCatalogLogicCognitionSurfaceAfterHerd proves Analyzer graph composition on
a multi-symbol herd tape through stack.Boot + Crypto.Tick.
*/
func TestCatalogLogicCognitionSurfaceAfterHerd(t *testing.T) {
	entry := catalog.MustKind(t, catalog.KindSectorLift)

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

	thesis := session.Crypto.LastThesis()

	if thesis == nil {
		t.Fatal("catalog logic: nil LastThesis")
	}

	graphs := 0

	thesis.Graphs.Range(func(_, value any) bool {
		evidenceGraph, ok := value.(*types.Graph)

		if ok && len(evidenceGraph.Edges()) > 0 {
			graphs++
		}

		return true
	})

	if graphs == 0 {
		t.Fatal("catalog logic: want at least one composed graph after herd tape")
	}
}

func BenchmarkCatalogLogicPumpGraphs(b *testing.B) {
	entry := catalog.MustKind(b, catalog.KindPump)
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

		thesis := session.Crypto.LastThesis()

		if thesis == nil {
			b.Fatal("nil LastThesis")
		}

		if _, ok := thesis.Graphs.Load(entry.Symbol); !ok {
			b.Fatal("missing graph")
		}
	}
}
