package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testtypes "github.com/theapemachine/symm/tests/types"
)

const (
	artifactDirectoryMode = os.FileMode(0o755)
	artifactFileMode      = os.FileMode(0o644)
)

/*
RegimeObservation records the oracle-only latent state on the shared timeline.
*/
type RegimeObservation struct {
	Tick   uint64                `json:"tick"`
	Symbol string                `json:"symbol"`
	State  testtypes.MarketState `json:"state"`
}

/*
SimulatorReport is the replayable separation of transport mechanics, order
mechanics, and execution economics for one scenario.
*/
type SimulatorReport struct {
	Scenario         testtypes.ScenarioConfig                    `json:"scenario"`
	Tick             uint64                                      `json:"tick"`
	Timeline         []RegimeObservation                         `json:"timeline"`
	RegimeExposure   map[string]map[testtypes.MarketState]uint64 `json:"regime_exposure"`
	PublicTransport  TransportReport                             `json:"public_transport"`
	PrivateTransport TransportReport                             `json:"private_transport"`
	Level3Transport  TransportReport                             `json:"level3_transport"`
	Mechanics        MechanicsReport                             `json:"mechanics"`
	Economics        EconomicsReport                             `json:"economics"`
}

/*
Report returns a detached snapshot suitable for assertions or persistence.
*/
func (market *Market) Report() SimulatorReport {
	report := SimulatorReport{
		Scenario:         market.Config.Clone(),
		Tick:             market.tick,
		Timeline:         append([]RegimeObservation{}, market.timeline...),
		RegimeExposure:   copyExposure(market.exposure),
		PublicTransport:  market.Public.faults.Report(),
		PrivateTransport: market.Private.faults.Report(),
		Level3Transport:  market.Level3.faults.Report(),
	}

	if market.execution != nil {
		report.Mechanics, report.Economics = market.execution.Report()
	}

	return report
}

/*
Validate checks simulator-owned invariants without assuming consumer behavior.
*/
func (market *Market) Validate() error {
	if market.execution == nil {
		return nil
	}

	return market.execution.Validate()
}

/*
WriteArtifact persists the complete configuration, timeline, injected faults,
generated frames, lifecycle, and accounting report for exact reproduction.
*/
func (market *Market) WriteArtifact(path string) error {
	if path == "" {
		return fmt.Errorf("simulator: artifact path is required")
	}

	payload, err := json.MarshalIndent(market.Report(), "", "  ")

	if err != nil {
		return fmt.Errorf("simulator: encode artifact: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), artifactDirectoryMode); err != nil {
		return fmt.Errorf("simulator: create artifact directory: %w", err)
	}

	if err := os.WriteFile(path, payload, artifactFileMode); err != nil {
		return fmt.Errorf("simulator: write artifact: %w", err)
	}

	return nil
}

/*
WithScenario runs a validated configured market and persists any failing run.
*/
func WithScenario(
	t *testing.T,
	config testtypes.ScenarioConfig,
	f func(*Market),
) func() {
	return func() {
		market, err := NewMarketWithScenario(t.Context(), config)

		if err != nil {
			t.Fatal(err)
			return
		}

		defer market.Close()
		defer func() {
			if !t.Failed() {
				return
			}

			name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
			stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
			path := filepath.Join(
				config.ArtifactDirectory,
				fmt.Sprintf("%s-seed-%d-%s.json", name, config.Seed, stamp),
			)

			if writeErr := market.WriteArtifact(path); writeErr != nil {
				t.Errorf("simulator: preserve failed scenario: %v", writeErr)
			}
		}()

		f(market)
	}
}

func copyExposure(
	source map[string]map[testtypes.MarketState]uint64,
) map[string]map[testtypes.MarketState]uint64 {
	copy := make(map[string]map[testtypes.MarketState]uint64, len(source))

	for symbol, states := range source {
		copy[symbol] = make(map[testtypes.MarketState]uint64, len(states))

		for state, ticks := range states {
			copy[symbol][state] = ticks
		}
	}

	return copy
}
