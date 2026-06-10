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
