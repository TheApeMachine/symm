package causal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	viper.SetConfigType("yml")

	for _, configPath := range []string{
		"cmd/cfg/config.yml",
		filepath.Join("..", "..", "cmd", "cfg", "config.yml"),
	} {
		viper.SetConfigFile(configPath)

		if viper.ReadInConfig() == nil {
			break
		}
	}

	os.Exit(m.Run())
}
