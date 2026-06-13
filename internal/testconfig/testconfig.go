package testconfig

import (
	"log"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
)

func configDir() string {
	_, file, _, ok := runtime.Caller(0)

	if !ok {
		log.Fatal("testconfig: runtime.Caller failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	return filepath.Join(repoRoot, "cmd", "cfg")
}

func loadMergedConfig() error {
	viper.Reset()
	viper.SetConfigType("yaml")
	viper.SetConfigFile(filepath.Join(configDir(), "config.yml"))

	return viper.ReadInConfig()
}

/*
MustLoad reads cmd/cfg/config.yml into the process-wide viper instance.
It fatals on failure and is intended for TestMain setup.
*/
func MustLoad() {
	if err := loadMergedConfig(); err != nil {
		log.Fatalf("testconfig: load config: %v", err)
	}
}

/*
Load reads cmd/cfg/config.yml into viper for one test.
*/
func Load(test *testing.T) {
	test.Helper()

	if err := loadMergedConfig(); err != nil {
		test.Fatalf("testconfig: load config: %v", err)
	}
}

/*
SeedRegimeDefaults sets the minimum regime config that every signal needs at
construction. NewSignal calls market.MustSignalMeasurementCapacity, which reads
regime.window / regime.baseline.min_obs and PANICS when they are unset — so any
signal unit test that only sets its own signals.* keys would otherwise abort.

Unlike Load it does NOT reset viper, so callers can set their signal-specific
keys before or after this call. Values mirror cmd/cfg/config.yml.
*/
func SeedRegimeDefaults() {
	// window=16, min_obs=4 yields BaseMeasurementCapacity == 4 (window/4),
	// matching the small ring sizes signal unit tests assert against. The
	// production config (cmd/cfg/config.yml) uses larger values; load it via
	// Load when a test needs production-scale capacity.
	if viper.GetInt("regime.window") <= 0 {
		viper.Set("regime.window", 16)
	}

	if viper.GetInt("regime.baseline.min_obs") <= 0 {
		viper.Set("regime.baseline.min_obs", 4)
	}
}
