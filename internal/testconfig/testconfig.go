package testconfig

import (
	"log"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
)

func configPath() string {
	_, file, _, ok := runtime.Caller(0)

	if !ok {
		log.Fatal("testconfig: runtime.Caller failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	return filepath.Join(repoRoot, "cmd", "cfg", "config.yml")
}

/*
MustLoad reads cmd/cfg/config.yml into the process-wide viper instance.
It fatals on failure and is intended for TestMain setup.
*/
func MustLoad() {
	viper.SetConfigType("yaml")
	viper.SetConfigFile(configPath())

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("testconfig: read %s: %v", configPath(), err)
	}
}

/*
Load reads cmd/cfg/config.yml into viper for one test.
*/
func Load(test *testing.T) {
	test.Helper()

	viper.SetConfigType("yaml")
	viper.SetConfigFile(configPath())

	if err := viper.ReadInConfig(); err != nil {
		test.Fatalf("testconfig: read %s: %v", configPath(), err)
	}
}
