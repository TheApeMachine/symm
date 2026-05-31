package config

/*
ScopedRuntime is an isolated view of Config decomposed into domain scopes.
Subsystems receive a ScopedRuntime pointer at construction instead of reading
config.System on hot paths.
*/
type ScopedRuntime struct {
	cfg       *Config
	Execution ExecutionScope
	Signal    SignalScope
	UI        UIScope
	Risk      RiskScope
}

// Runtime is the process-wide scoped config injected into subsystems at boot.
var Runtime *ScopedRuntime

/*
NewRuntime builds scope copies from cfg.
*/
func NewRuntime(cfg *Config) *ScopedRuntime {
	return &ScopedRuntime{
		cfg:       cfg,
		Execution: ExecutionScopeFrom(cfg),
		Signal:    SignalScopeFrom(cfg),
		UI:        UIScopeFrom(cfg),
		Risk:      RiskScopeFrom(cfg),
	}
}

/*
Refresh rebuilds scope copies after cfg mutates.
*/
func (runtime *ScopedRuntime) Refresh(cfg *Config) {
	if runtime == nil {
		return
	}

	runtime.cfg = cfg
	runtime.Execution = ExecutionScopeFrom(cfg)
	runtime.Signal = SignalScopeFrom(cfg)
	runtime.UI = UIScopeFrom(cfg)
	runtime.Risk = RiskScopeFrom(cfg)
}

/*
Config returns the backing config pointer.
*/
func (runtime *ScopedRuntime) Config() *Config {
	if runtime == nil {
		return System
	}

	return runtime.cfg
}

/*
SyncRuntime rebuilds Runtime after System changes.
*/
func SyncRuntime() {
	if Runtime == nil {
		Runtime = NewRuntime(System)

		return
	}

	Runtime.Refresh(System)
}
