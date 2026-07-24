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

	return config, nil
}

/*
Fixture returns a deterministic config snapshot for unit and integration tests.
It does not read Viper so tests stay isolated from process-wide config state.
*/
func Fixture() Config {
	return Config{
		System: SystemConfig{
			ActorBuffer:   64,
			ChannelBuffer: 128,
			DataPath:        "/tmp/symm-test",
			AuditRotate:     false,
		},
		Market: MarketConfig{
			QuoteCurrency:  "USD",
			SubscribeBatch: 10,
			L3Enabled:      true,
			L3Depth:        10,
			L3RateLimit:    200,
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
			FeedTrackCapacity: 512,
		},
	}
}
