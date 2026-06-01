package causal

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	configLoaded := false

	for _, configPath := range []string{
		"cmd/cfg/config.yml",
		filepath.Join("..", "..", "cmd", "cfg", "config.yml"),
	} {
		viper.SetConfigFile(configPath)

		if viper.ReadInConfig() == nil {
			configLoaded = true

			break
		}
	}

	if !configLoaded {
		log.Fatalf("causal tests require config.yml (tried cmd/cfg/config.yml and ../../cmd/cfg/config.yml)")
	}

	os.Exit(m.Run())
}
