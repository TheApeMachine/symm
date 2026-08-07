package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

/*
Config is the immutable process configuration loaded once at startup.
Constructors receive typed slices of this value; live file watching is not
enabled until an atomic generation swap exists.
*/
type Config struct {
	System    SystemConfig
	Market    MarketConfig
	Trading   TradingConfig
	UI        UIConfig
	Signals   SignalsConfig
	Cognitive CognitiveConfig
}

/*
SystemConfig holds process-wide runtime knobs.
*/
type SystemConfig struct {
	DataPath        string
	ActorBuffer     int
	ChannelBuffer   int
	LogLevel        string
	AuditRotate     bool
	CheckpointEvery time.Duration
}

/*
MarketConfig holds venue and signal-feed settings.
*/
type MarketConfig struct {
	QuoteCurrency    string
	SubscribeBatch   int
	SubscribePace    time.Duration
	L3Enabled        bool
	L3Depth          int
	L3RateLimit      int
	BaselineHalflife time.Duration
}

/*
TradingConfig holds allocation and execution mode.
*/
type TradingConfig struct {
	Model             string
	MaxFraction       float64
	MinimumConfidence float64
	SlotsNormal       int
	SlotsReserved     int
	Risk              RiskConfig
}

/*
RiskConfig is the stop geometry every entry is sized under.

It is validated rather than defaulted at the point of use, because the failure
mode is silent and expensive: a loss fraction of zero read through a permissive
accessor does not disable the cap, it makes the whole wallet the budget for a
single lot.
*/
type RiskConfig struct {
	/*
		MaxLossFraction is what one position may lose at its hard floor, as a
		fraction of the wallet. PortfolioLossFraction is the same for every open
		position taken together.

		Both exist because the per-position number alone says nothing about the
		account. Four simultaneous entries at one percent each are a four
		percent account risk, and nothing in a per-position limit notices.
	*/
	MaxLossFraction       float64
	PortfolioLossFraction float64
	NoiseMultiple         float64
	TrailMultiple         float64
	ArmMultiple           float64
	LockMultiple          float64
	MinEdgeMultiple       float64
	MinTicks              int
	ConfirmMarks          int
}

/*
UIConfig holds the local dashboard bind address.
*/
type UIConfig struct {
	Addr string
}

/*
SignalsConfig holds feed retention capacities.
*/
type SignalsConfig struct {
	FeedTimelineCapacity int
	FeedTrackCapacity    int
}

/*
CognitiveConfig holds DMT store options.
*/
type CognitiveConfig struct {
	InMemory   bool
	TickBudget time.Duration
}

/*
Load reads the already-initialized Viper tree into an immutable Config snapshot.
Callers must not mutate the returned value.
*/
func Load() (Config, error) {
	config := Config{
		System: SystemConfig{
			DataPath:        viper.GetString("system.data_path"),
			ActorBuffer:     viper.GetInt("system.actor.buffer"),
			ChannelBuffer:   viper.GetInt("system.websocket.channel.buffer"),
			LogLevel:        viper.GetString("system.log.level"),
			AuditRotate:     viper.GetBool("system.audit.rotate_on_boot"),
			CheckpointEvery: viper.GetDuration("system.checkpoint_interval"),
		},
		Market: MarketConfig{
			QuoteCurrency:    viper.GetString("market.quote_currency"),
			SubscribeBatch:   viper.GetInt("market.subscribe_batch"),
			SubscribePace:    viper.GetDuration("market.subscribe_pace"),
			L3Enabled:        viper.GetBool("market.l3_enabled"),
			L3Depth:          viper.GetInt("market.l3_depth"),
			L3RateLimit:      viper.GetInt("market.l3_rate_limit"),
			BaselineHalflife: viper.GetDuration("market.baseline_halflife"),
		},
		Trading: TradingConfig{
			Model:             viper.GetString("trading.model"),
			MaxFraction:       viper.GetFloat64("trading.allocation.max_fraction"),
			MinimumConfidence: viper.GetFloat64("trading.resonance.minimum_confidence"),
			SlotsNormal:       viper.GetInt("trading.slots.normal"),
			SlotsReserved:     viper.GetInt("trading.slots.reserved"),
			Risk: RiskConfig{
				MaxLossFraction:       viper.GetFloat64("trading.risk.max_loss_fraction"),
				PortfolioLossFraction: viper.GetFloat64("trading.risk.portfolio_loss_fraction"),
				NoiseMultiple:         viper.GetFloat64("trading.risk.noise_multiple"),
				TrailMultiple:         viper.GetFloat64("trading.risk.trail_multiple"),
				ArmMultiple:           viper.GetFloat64("trading.risk.arm_multiple"),
				LockMultiple:          viper.GetFloat64("trading.risk.lock_multiple"),
				MinEdgeMultiple:       viper.GetFloat64("trading.risk.min_edge_multiple"),
				MinTicks:              viper.GetInt("trading.risk.min_ticks"),
				ConfirmMarks:          viper.GetInt("trading.risk.confirm_marks"),
			},
		},
		UI: UIConfig{
			Addr: viper.GetString("ui.addr"),
		},
		Signals: SignalsConfig{
			FeedTimelineCapacity: viper.GetInt("signals.feed_timeline_capacity"),
			FeedTrackCapacity:    viper.GetInt("signals.feed_track_capacity"),
		},
		Cognitive: CognitiveConfig{
			InMemory:   viper.GetBool("cognitive.in_memory"),
			TickBudget: viper.GetDuration("cognitive.tick_budget"),
		},
	}

	if config.System.ActorBuffer < 1 {
		return Config{}, fmt.Errorf("config: system.actor.buffer must be >= 1")
	}

	if config.System.ChannelBuffer < 1 {
		return Config{}, fmt.Errorf("config: system.websocket.channel.buffer must be >= 1")
	}

	if config.UI.Addr == "" {
		return Config{}, fmt.Errorf("config: ui.addr is required")
	}

	if config.Market.QuoteCurrency == "" {
		return Config{}, fmt.Errorf("config: market.quote_currency is required")
	}

	if config.Market.SubscribeBatch < 1 {
		return Config{}, fmt.Errorf("config: market.subscribe_batch must be >= 1")
	}

	if config.Market.SubscribePace <= 0 {
		return Config{}, fmt.Errorf("config: market.subscribe_pace must be > 0")
	}

	if config.Market.BaselineHalflife <= 0 {
		return Config{}, fmt.Errorf("config: market.baseline_halflife must be > 0")
	}

	if config.Trading.MaxFraction <= 0 || config.Trading.MaxFraction > 1 {
		return Config{}, fmt.Errorf("config: trading.allocation.max_fraction must be in (0, 1]")
	}

	if config.Trading.MinimumConfidence <= 0 ||
		config.Trading.MinimumConfidence > 1 {
		return Config{}, fmt.Errorf(
			"config: trading.resonance.minimum_confidence must be in (0, 1]",
		)
	}

	if config.Trading.SlotsNormal < 1 {
		return Config{}, fmt.Errorf("config: trading.slots.normal must be >= 1")
	}

	if config.Trading.SlotsReserved < 0 {
		return Config{}, fmt.Errorf("config: trading.slots.reserved must be >= 0")
	}

	if err := config.Trading.Risk.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

/*
validate refuses a risk block that would silently disable the protection it
describes.

Every check here corresponds to a way the geometry degenerates rather than
erroring: a zero loss fraction makes the whole wallet one lot's budget, a
non-positive noise multiple puts the hard floor on the entry price, an arm
buffer at or below the lock buffer lets protection arm and fire on the same
tick, and a confirmation count below one turns every wick into an exit.
*/
func (risk RiskConfig) Validate() error {
	if risk.MaxLossFraction <= 0 || risk.MaxLossFraction > 1 {
		return fmt.Errorf("config: trading.risk.max_loss_fraction must be in (0, 1]")
	}

	if risk.PortfolioLossFraction <= 0 || risk.PortfolioLossFraction > 1 {
		return fmt.Errorf("config: trading.risk.portfolio_loss_fraction must be in (0, 1]")
	}

	if risk.PortfolioLossFraction < risk.MaxLossFraction {
		return fmt.Errorf(
			"config: trading.risk.portfolio_loss_fraction must be >= max_loss_fraction",
		)
	}

	if risk.NoiseMultiple <= 0 {
		return fmt.Errorf("config: trading.risk.noise_multiple must be > 0")
	}

	if risk.TrailMultiple <= 0 {
		return fmt.Errorf("config: trading.risk.trail_multiple must be > 0")
	}

	if risk.LockMultiple <= 0 {
		return fmt.Errorf("config: trading.risk.lock_multiple must be > 0")
	}

	if risk.ArmMultiple <= risk.LockMultiple {
		return fmt.Errorf("config: trading.risk.arm_multiple must be > lock_multiple")
	}

	if risk.MinEdgeMultiple <= 0 {
		return fmt.Errorf("config: trading.risk.min_edge_multiple must be > 0")
	}

	if risk.MinTicks < 1 {
		return fmt.Errorf("config: trading.risk.min_ticks must be >= 1")
	}

	if risk.ConfirmMarks < 1 {
		return fmt.Errorf("config: trading.risk.confirm_marks must be >= 1")
	}

	return nil
}

/*
Fixture returns a deterministic config snapshot for unit and integration tests.
It does not read Viper so tests stay isolated from process-wide config state.
*/
func Fixture() Config {
	return Config{
		System: SystemConfig{
			ActorBuffer:     64,
			ChannelBuffer:   4096,
			DataPath:        "/tmp/symm-test",
			AuditRotate:     false,
			CheckpointEvery: time.Second,
		},
		Market: MarketConfig{
			QuoteCurrency:    "USD",
			SubscribeBatch:   10,
			SubscribePace:    20 * time.Millisecond,
			L3Enabled:        true,
			L3Depth:          10,
			L3RateLimit:      200,
			BaselineHalflife: 30 * time.Second,
		},
		Trading: TradingConfig{
			Model:             "paper",
			MaxFraction:       0.2,
			MinimumConfidence: 0.8,
			SlotsNormal:       2,
			SlotsReserved:     2,
			Risk: RiskConfig{
				MaxLossFraction:       0.01,
				PortfolioLossFraction: 0.03,
				NoiseMultiple:         3,
				TrailMultiple:         2,
				ArmMultiple:           2,
				LockMultiple:          1,
				MinEdgeMultiple:       1,
				MinTicks:              4,
				ConfirmMarks:          3,
			},
		},
		UI: UIConfig{
			Addr: "127.0.0.1:0",
		},
		Signals: SignalsConfig{
			FeedTimelineCapacity: 4096,
			FeedTrackCapacity:    512,
		},
		Cognitive: CognitiveConfig{
			InMemory:   true,
			TickBudget: 10 * time.Millisecond,
		},
	}
}
