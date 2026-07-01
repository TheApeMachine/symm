package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market"
)

/*
NewTestPool allocates a small qpool for signal tests.
*/
func NewTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

/*
NewTestCrossSection builds a cross-section with test sizing defaults.
*/
func NewTestCrossSection(testingTB testing.TB) *market.CrossSection {
	if testingTB != nil {
		testingTB.Helper()
	}

	section, err := market.NewCrossSection(
		datura.Acquire("test", datura.APPJSON).
			WithRole("cross_section_config").
			Poke(float64(16), "return_cap").
			Poke(float64(6), "min_bars").
			Poke(float64(16), "breadth_hist"),
	)

	if err != nil && testingTB != nil {
		testingTB.Fatal(err)
	}

	return section
}

/*
TickerDatapoint builds one Kraken ticker ingest frame.
*/
func TickerDatapoint(symbol string, last, changePct float64, timestamp int64) *datura.Artifact {
	return TickerDatapointWithVolume(symbol, last, 1000, changePct, timestamp)
}

/*
TickerDatapointWithVolume builds one Kraken ticker ingest frame with volume.
*/
func TickerDatapointWithVolume(
	symbol string,
	last, volume, changePct float64,
	timestamp int64,
) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"volume":%g,"change_pct":%g}]}`,
		symbol, last, volume, changePct,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

/*
TickerDatapointAt builds a ticker frame from a wall-clock time.
*/
func TickerDatapointAt(symbol string, last, changePct float64, at time.Time) *datura.Artifact {
	return TickerDatapoint(symbol, last, changePct, at.UnixNano())
}
