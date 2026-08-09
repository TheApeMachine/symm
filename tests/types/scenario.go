package types

import (
	"fmt"
	"math"
	"time"
)

const (
	DefaultScenarioSeed       = int64(1)
	DefaultCandleInterval     = time.Minute
	DefaultBookApplyTimeout   = 5 * time.Second
	DefaultBookPollInterval   = 10 * time.Millisecond
	DefaultFlattenTickLimit   = 512
	DefaultDepthLevels        = 1
	DefaultDepthQuantityScale = 1.0
	DefaultLimitFillProb      = 1.0
	BasisPointDenominator     = 10_000.0
	MaximumExactJSONInteger   = uint64(1<<53 - 1)
	DefaultArtifactDirectory  = "runs/simulator-failures"
)

/* DefaultScenarioStart anchors byte-identical generated timestamps. */
var DefaultScenarioStart = time.Unix(0, 0).UTC()

/*
OrderOutcome selects a deterministic terminal path before probability-based
outcomes are sampled. An empty outcome leaves selection to ExecutionConfig.
*/
type OrderOutcome string

const (
	OrderFill   OrderOutcome = "fill"
	OrderReject OrderOutcome = "reject"
	OrderCancel OrderOutcome = "cancel"
	OrderNoFill OrderOutcome = "no_fill"
	OrderExpire OrderOutcome = "expire"
)

/*
ExecutionConfig parameterizes the stateful simulated execution venue.
*/
type ExecutionConfig struct {
	DepthLevels                   int
	DepthQuantityScale            float64
	SlippageBasisPoints           float64
	ExecutionDelay                time.Duration
	AcknowledgementDelay          time.Duration
	PartialFillProb               float64
	MeanFragmentCount             float64
	FragmentDelay                 time.Duration
	RejectionProb                 float64
	CancellationProb              float64
	NoFillProb                    float64
	LimitFillProb                 float64
	ExpireAfter                   time.Duration
	BalanceDelay                  time.Duration
	RESTBalanceDelay              time.Duration
	MaximumOrderQuantity          float64
	EnforceBalances               bool
	EmitAcknowledgements          bool
	ExecutionBeforeAcknowledgment bool
	Outcomes                      []OrderOutcome
}

/*
DefaultExecutionConfig preserves the original next-tick, top-of-book market
fill behavior while exposing every source of friction for explicit scenarios.
*/
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		DepthLevels:        DefaultDepthLevels,
		DepthQuantityScale: DefaultDepthQuantityScale,
		MeanFragmentCount:  1,
		LimitFillProb:      DefaultLimitFillProb,
		EnforceBalances:    true,
	}
}

/*
Validate rejects execution settings that cannot represent a coherent venue.
*/
func (config ExecutionConfig) Validate() error {
	if config.DepthLevels < 1 {
		return fmt.Errorf("scenario: execution depth levels must be positive")
	}

	if config.DepthQuantityScale <= 0 ||
		math.IsNaN(config.DepthQuantityScale) ||
		math.IsInf(config.DepthQuantityScale, 0) {
		return fmt.Errorf("scenario: execution depth quantity scale must be positive and finite")
	}

	if config.SlippageBasisPoints < 0 ||
		math.IsNaN(config.SlippageBasisPoints) ||
		math.IsInf(config.SlippageBasisPoints, 0) {
		return fmt.Errorf("scenario: execution slippage must be finite and non-negative")
	}

	if config.ExecutionDelay < 0 || config.AcknowledgementDelay < 0 ||
		config.FragmentDelay < 0 || config.ExpireAfter < 0 ||
		config.BalanceDelay < 0 || config.RESTBalanceDelay < 0 {
		return fmt.Errorf("scenario: execution delays must be non-negative")
	}

	probabilities := map[string]float64{
		"partial fill": config.PartialFillProb,
		"rejection":    config.RejectionProb,
		"cancellation": config.CancellationProb,
		"no fill":      config.NoFillProb,
		"limit fill":   config.LimitFillProb,
	}

	for name, probability := range probabilities {
		if probability < 0 || probability > 1 || math.IsNaN(probability) {
			return fmt.Errorf("scenario: %s probability must be between zero and one", name)
		}
	}

	terminalProbability := config.RejectionProb + config.CancellationProb +
		config.NoFillProb

	if terminalProbability > 1 {
		return fmt.Errorf("scenario: terminal order outcome probabilities exceed one")
	}

	if config.PartialFillProb > 0 && config.MeanFragmentCount < 2 {
		return fmt.Errorf("scenario: partial fills require a mean fragment count of at least two")
	}

	if config.MeanFragmentCount < 1 || math.IsNaN(config.MeanFragmentCount) ||
		math.IsInf(config.MeanFragmentCount, 0) {
		return fmt.Errorf("scenario: mean fragment count must be positive and finite")
	}

	if config.MaximumOrderQuantity < 0 || math.IsNaN(config.MaximumOrderQuantity) ||
		math.IsInf(config.MaximumOrderQuantity, 0) {
		return fmt.Errorf("scenario: maximum order quantity must be finite and non-negative")
	}

	for _, outcome := range config.Outcomes {
		switch outcome {
		case OrderFill, OrderReject, OrderCancel, OrderNoFill, OrderExpire:
		default:
			return fmt.Errorf("scenario: unknown order outcome %q", outcome)
		}
	}

	return nil
}

/*
FaultAction names one deterministic transport disruption.
*/
type FaultAction string

const (
	FaultDrop        FaultAction = "drop"
	FaultDuplicate   FaultAction = "duplicate"
	FaultDelay       FaultAction = "delay"
	FaultReorder     FaultAction = "reorder"
	FaultSequenceGap FaultAction = "sequence_gap"
	FaultStale       FaultAction = "stale"
	FaultMalformed   FaultAction = "malformed"
	FaultReconnect   FaultAction = "reconnect"
)

/*
LatencyConfig defines a seeded, bounded arrival delay.
*/
type LatencyConfig struct {
	Base   time.Duration
	Jitter time.Duration
}

/*
FaultRule applies one action to the declared occurrence of a channel frame.
*/
type FaultRule struct {
	Channel     string
	Occurrence  int
	Action      FaultAction
	Delay       time.Duration
	SequenceGap uint64
	Payload     []byte
}

/*
FaultConfig controls frame delivery, REST timing, and reconnection faults.
*/
type FaultConfig struct {
	Seed           int64
	ChannelLatency map[string]LatencyConfig
	RESTLatency    map[string]LatencyConfig
	Rules          []FaultRule
}

/*
Validate rejects fault rules whose trigger or timing is ambiguous.
*/
func (config FaultConfig) Validate() error {
	for channel, latency := range config.ChannelLatency {
		if channel == "" {
			return fmt.Errorf("scenario: channel latency requires a channel")
		}

		if latency.Base < 0 || latency.Jitter < 0 {
			return fmt.Errorf("scenario: channel %s latency must be non-negative", channel)
		}
	}

	for path, latency := range config.RESTLatency {
		if path == "" {
			return fmt.Errorf("scenario: REST latency requires a path")
		}

		if latency.Base < 0 || latency.Jitter < 0 {
			return fmt.Errorf("scenario: REST %s latency must be non-negative", path)
		}
	}

	seen := map[string]struct{}{}

	for _, rule := range config.Rules {
		if rule.Channel == "" || rule.Occurrence < 1 {
			return fmt.Errorf("scenario: fault rules require a channel and positive occurrence")
		}

		switch rule.Action {
		case FaultDrop, FaultDuplicate, FaultReorder, FaultStale,
			FaultMalformed, FaultReconnect:
		case FaultDelay:
			if rule.Delay <= 0 {
				return fmt.Errorf("scenario: delay fault requires a positive delay")
			}
		case FaultSequenceGap:
			if rule.SequenceGap < 1 ||
				rule.SequenceGap > MaximumExactJSONInteger {
				return fmt.Errorf("scenario: sequence gap fault requires a positive exact JSON integer gap")
			}
		default:
			return fmt.Errorf("scenario: unknown fault action %q", rule.Action)
		}

		identity := fmt.Sprintf("%s:%d", rule.Channel, rule.Occurrence)

		if _, exists := seen[identity]; exists {
			return fmt.Errorf("scenario: duplicate fault trigger %s", identity)
		}

		seen[identity] = struct{}{}
	}

	return nil
}

/*
RegimeTransition schedules one latent regime change on the shared tick line.
*/
type RegimeTransition struct {
	Tick   uint64
	Symbol string
	State  MarketState
}

/*
ScenarioConfig is the complete replay identity of a simulated run.
*/
type ScenarioConfig struct {
	Name              string
	Seed              int64
	StartTime         time.Time
	Symbols           []*Symbol
	Profiles          map[MarketState]RegimeProfile
	Momentum          map[MarketState]float64
	CandleInterval    time.Duration
	BookApplyTimeout  time.Duration
	BookPollInterval  time.Duration
	FlattenTickLimit  int
	Schedule          []RegimeTransition
	Execution         ExecutionConfig
	Faults            FaultConfig
	InitialBalances   map[string]float64
	ArtifactDirectory string
}

/*
NewScenarioConfig creates a deterministic mechanics scenario for the supplied
symbols. Generator seeds remain attached to each symbol; Seed controls shared
market factors, fault jitter, and execution outcomes.
*/
func NewScenarioConfig(symbols []*Symbol) ScenarioConfig {
	return ScenarioConfig{
		Name:             "deterministic-mechanics",
		Seed:             DefaultScenarioSeed,
		StartTime:        DefaultScenarioStart,
		Symbols:          symbols,
		Profiles:         CloneProfiles(DefaultProfiles),
		Momentum:         CloneMomentum(MomentumMap),
		CandleInterval:   DefaultCandleInterval,
		BookApplyTimeout: DefaultBookApplyTimeout,
		BookPollInterval: DefaultBookPollInterval,
		FlattenTickLimit: DefaultFlattenTickLimit,
		Execution:        DefaultExecutionConfig(),
		Faults: FaultConfig{
			Seed: DefaultScenarioSeed,
		},
		ArtifactDirectory: DefaultArtifactDirectory,
	}
}

/*
Validate confirms that a scenario can be reproduced without hidden defaults or
ambiguous symbol and transition identities.
*/
func (config ScenarioConfig) Validate() error {
	if config.Name == "" {
		return fmt.Errorf("scenario: name is required")
	}

	if config.StartTime.IsZero() {
		return fmt.Errorf("scenario: start time is required")
	}

	if len(config.Symbols) == 0 {
		return fmt.Errorf("scenario: at least one symbol is required")
	}

	if config.CandleInterval <= 0 || config.CandleInterval%time.Minute != 0 {
		return fmt.Errorf("scenario: candle interval must be a positive whole minute")
	}

	if config.BookApplyTimeout <= 0 || config.BookPollInterval <= 0 ||
		config.BookPollInterval > config.BookApplyTimeout {
		return fmt.Errorf("scenario: book synchronization timing must be positive and bounded")
	}

	if config.FlattenTickLimit < 1 {
		return fmt.Errorf("scenario: flatten tick limit must be positive")
	}

	if config.ArtifactDirectory == "" {
		return fmt.Errorf("scenario: artifact directory is required")
	}

	if err := validateProfiles(config.Profiles, config.Momentum); err != nil {
		return err
	}

	symbols := make(map[string]struct{}, len(config.Symbols))

	for _, symbol := range config.Symbols {
		if err := validateSymbol(symbol); err != nil {
			return err
		}

		if _, exists := symbols[symbol.Pair]; exists {
			return fmt.Errorf("scenario: duplicate symbol %q", symbol.Pair)
		}

		symbols[symbol.Pair] = struct{}{}
	}

	if err := config.Execution.Validate(); err != nil {
		return err
	}

	if err := config.Faults.Validate(); err != nil {
		return err
	}

	transitions := map[string]struct{}{}

	for _, transition := range config.Schedule {
		if _, exists := symbols[transition.Symbol]; !exists {
			return fmt.Errorf("scenario: transition references unknown symbol %q", transition.Symbol)
		}

		if _, exists := config.Profiles[transition.State]; !exists {
			return fmt.Errorf("scenario: transition references unknown state %d", transition.State)
		}

		identity := fmt.Sprintf("%s:%d", transition.Symbol, transition.Tick)

		if _, exists := transitions[identity]; exists {
			return fmt.Errorf("scenario: duplicate transition %s", identity)
		}

		transitions[identity] = struct{}{}
	}

	for asset, balance := range config.InitialBalances {
		if asset == "" || balance < 0 || math.IsNaN(balance) || math.IsInf(balance, 0) {
			return fmt.Errorf("scenario: initial balances require named assets and finite non-negative amounts")
		}
	}

	return nil
}
