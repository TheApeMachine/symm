package causal

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)

	if !ok {
		log.Fatal("causal TestMain: runtime.Caller failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	configPath := filepath.Join(repoRoot, "cmd", "cfg", "config.yml")

	viper.SetConfigType("yaml")
	viper.SetConfigFile(configPath)

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("causal TestMain: read %s: %v", configPath, err)
	}

	if viper.GetViper().GetFloat64("signals.causal.contagion_break") <= 0 {
		log.Fatalf("causal TestMain: missing signals.causal.contagion_break in %s", configPath)
	}

	os.Exit(m.Run())
}
