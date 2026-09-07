package market

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"time"
)

/*
LeadLagTape replays the CRV/DOT observations captured on 2026-09-07 that
stalled the spot workload. Uneven nanosecond spacing placed the winning lag
at the profile boundary; converting its seconds back into an index rounded
that boundary inward and attempted to read a negative neighbour.
*/
func LeadLagTape() []kraken.TickerData {
	observations := []struct{ symbol, last, at string }{
		{"CRV/USD", "0.38371", "2026-09-07T12:38:51.994838Z"},
		{"DOT/USD", "0.9967", "2026-09-07T12:38:52.742285Z"},
		{"DOT/USD", "0.9977", "2026-09-07T12:38:54.866024Z"},
		{"DOT/USD", "0.9977", "2026-09-07T12:38:54.866950Z"},
		{"DOT/USD", "0.9977", "2026-09-07T12:38:54.888887Z"},
		{"DOT/USD", "0.9983", "2026-09-07T12:38:54.984366Z"},
		{"DOT/USD", "0.9977", "2026-09-07T12:39:06.124864Z"},
		{"DOT/USD", "0.9985", "2026-09-07T12:39:07.384456Z"},
		{"CRV/USD", "0.38305", "2026-09-07T12:39:07.915713Z"},
		{"CRV/USD", "0.38305", "2026-09-07T12:39:07.916818Z"},
		{"CRV/USD", "0.38283", "2026-09-07T12:39:07.917894Z"},
		{"CRV/USD", "0.3827", "2026-09-07T12:39:07.918750Z"},
		{"CRV/USD", "0.3827", "2026-09-07T12:39:07.919411Z"},
		{"CRV/USD", "0.3827", "2026-09-07T12:39:07.919508Z"},
		{"CRV/USD", "0.3828", "2026-09-07T12:39:07.923102Z"},
		{"CRV/USD", "0.38261", "2026-09-07T12:39:07.924929Z"},
		{"CRV/USD", "0.3826", "2026-09-07T12:39:07.925577Z"},
		{"CRV/USD", "0.3826", "2026-09-07T12:39:07.925882Z"},
		{"CRV/USD", "0.3826", "2026-09-07T12:39:07.926011Z"},
	}
	messages := make([]kraken.TickerData, len(observations))
	for index, observation := range observations {
		at, err := time.Parse(time.RFC3339Nano, observation.at)

		if err != nil {
			panic(err)
		}
		last, err := decimal.NewFromString(observation.last)

		if err != nil {
			panic(err)
		}
		messages[index] = kraken.TickerData{Symbol: observation.symbol, Last: last, Timestamp: at}
	}
	return messages
}
