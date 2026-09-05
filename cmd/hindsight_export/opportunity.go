package main

import (
	"encoding/json"
	"fmt"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
)

/* opportunityRecord carries one canonical Hindsight price episode. */
type opportunityRecord struct {
	Kind           string            `json:"kind"`
	Run            string            `json:"run"`
	Episode        hindsight.Episode `json:"episode"`
	ExtremumBid    float64           `json:"extremumBid,omitempty"`
	HasExtremumBid bool              `json:"hasExtremumBid"`
}

/* writeOpportunityRecords exports the same price geometry the Hindsight UI shows. */
func writeOpportunityRecords(
	engine *store.SQLite,
	runID string,
	encoder *json.Encoder,
) (int, error) {
	observations, err := engine.ListMarketObservations(runID)

	if err != nil {
		return 0, fmt.Errorf("list market observations: %w", err)
	}

	index := hindsight.NewRunIndex(hindsight.RunID(runID), observations)
	policy := hindsight.DefaultDiscoveryPolicy()
	quotes := make(map[hindsight.EnvelopeRef]float64)
	written := 0

	for _, observation := range observations {
		if !observation.HasBid || observation.Bid <= 0 {
			continue
		}

		quotes[hindsight.EnvelopeRef{
			Origin:  observation.Capture,
			Ordinal: observation.Ordinal,
		}] = observation.Bid
	}

	for _, summary := range index.Summaries(policy) {
		discovery := index.Discover(summary.Symbol, policy)

		for _, episode := range discovery.Episodes {
			if episode.Kind != hindsight.EpisodeUpwardExcursion &&
				episode.Kind != hindsight.EpisodeDownwardExcursion {
				continue
			}

			role := hindsight.ReferencePeak

			if episode.Kind == hindsight.EpisodeDownwardExcursion {
				role = hindsight.ReferenceTrough
			}

			extremum, found := episode.Reference(role)
			record := opportunityRecord{
				Kind:    "episode",
				Run:     runID,
				Episode: episode,
			}

			if found {
				record.ExtremumBid, record.HasExtremumBid = quotes[hindsight.EnvelopeRef{
					Origin:  extremum.Capture,
					Ordinal: extremum.Ordinal,
				}]
			}

			if err := encoder.Encode(record); err != nil {
				return written, fmt.Errorf("encode opportunity episode: %w", err)
			}

			written++
		}
	}

	return written, nil
}
