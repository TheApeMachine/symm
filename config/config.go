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
	Model         string
	MaxFraction   float64
	SlotsNormal   int
	SlotsReserved int
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
			Model:         viper.GetString("trading.model"),
			MaxFraction:   viper.GetFloat64("trading.allocation.max_fraction"),
			SlotsNormal:   viper.GetInt("trading.slots.normal"),
			SlotsReserved: viper.GetInt("trading.slots.reserved"),
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

	if config.Trading.SlotsNormal < 1 {
		return Config{}, fmt.Errorf("config: trading.slots.normal must be >= 1")
	}

	if config.Trading.SlotsReserved < 0 {
		return Config{}, fmt.Errorf("config: trading.slots.reserved must be >= 0")
	}

	return config, nil
}

/*
Fixture returns a deterministic config snapshot for unit and integration tests.
It does not read Viper so tests stay isolated from process-wide config state.
*/
func Fixture() Config {
	return Config{
		System: SystemConfig{
			ActorBuffer:     64,
			ChannelBuffer:   128,
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
			Model:         "paper",
			MaxFraction:   0.2,
			SlotsNormal:   2,
			SlotsReserved: 2,
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
