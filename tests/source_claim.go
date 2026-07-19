package tests

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/theapemachine/symm/types"
)

/*
MetricBound selects how RequireSourceClaims interprets PeakSourceMetric.
*/
type MetricBound string

const (
	// BoundPositive requires a found peak strictly greater than zero (and
	// optionally Claim.Min when set).
	BoundPositive MetricBound = "positive"
	// BoundPresent requires a found non-zero peak (signed metrics allowed).
	BoundPresent MetricBound = "present"
	// BoundZero requires a found peak exactly equal to zero.
	BoundZero MetricBound = "zero"
)

/*
SourceClaim is one absolute known outcome for a signal's Session.Play tape.
*/
type SourceClaim struct {
	Source types.SourceType
	Metric types.MetricType
	Symbol string
	Bound  MetricBound
	// Min is an optional absolute floor when Bound is BoundPositive.
	Min float64
}

/*
CheckSourceClaim returns an error when the claim fails against Play theses.
*/
func CheckSourceClaim(theses []*types.Thesis, claim SourceClaim) error {
	peak, ok := PeakSourceMetric(theses, claim.Source, claim.Symbol, claim.Metric)

	switch claim.Bound {
	case BoundPresent:
		if !ok || peak == 0 {
			return fmt.Errorf(
				"want non-zero %s/%s on %s (ok=%v peak=%g)",
				claim.Source, claim.Metric, claim.Symbol, ok, peak,
			)
		}
	case BoundZero:
		if !ok || peak != 0 {
			return fmt.Errorf(
				"want zero %s/%s on %s (ok=%v peak=%g)",
				claim.Source, claim.Metric, claim.Symbol, ok, peak,
			)
		}
	default:
		floor := claim.Min

		if !ok || peak <= 0 || peak < floor {
			return fmt.Errorf(
				"want positive %s/%s on %s (ok=%v peak=%g min=%g)",
				claim.Source, claim.Metric, claim.Symbol, ok, peak, floor,
			)
		}
	}

	return nil
}

/*
RequireSourceClaims fails the test when any absolute source/metric claim fails.
*/
func RequireSourceClaims(
	t testing.TB,
	theses []*types.Thesis,
	claims ...SourceClaim,
) {
	t.Helper()

	if len(theses) == 0 {
		t.Fatal("want theses from Session.Play")
	}

	for _, claim := range claims {
		if err := CheckSourceClaim(theses, claim); err != nil {
			t.Fatal(err)
		}
	}
}

/*
RequireSourceExceeds fails unless the stressed tape peak strictly exceeds calm
for the same source/symbol/metric (contrast after absolute claims hold).
*/
func RequireSourceExceeds(
	t testing.TB,
	stressed []*types.Thesis,
	calm []*types.Thesis,
	source types.SourceType,
	symbol string,
	metric types.MetricType,
) {
	t.Helper()

	hot, hasHot := PeakSourceMetric(stressed, source, symbol, metric)
	cold, hasCold := PeakSourceMetric(calm, source, symbol, metric)

	if !hasHot {
		t.Fatalf("want %s/%s on stressed %s", source, metric, symbol)
	}

	if !hasCold {
		t.Fatalf("want %s/%s on calm %s", source, metric, symbol)
	}

	if hot <= cold {
		t.Fatalf(
			"want stressed %s/%s (%g) > calm (%g) on %s",
			source, metric, hot, cold, symbol,
		)
	}
}

/*
PlayMarketClaims boots Session with signals, plays the tape, and asserts claims.
*/
func PlayMarketClaims(
	t testing.TB,
	options SessionOptions,
	frames iter.Seq[Frame],
	claims ...SourceClaim,
) []*types.Thesis {
	t.Helper()

	session, err := NewSession(context.Background(), t, options)

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	theses, err := session.Play(frames)

	if err != nil {
		t.Fatalf("play: %v", err)
	}

	RequireSourceClaims(t, theses, claims...)

	return theses
}
