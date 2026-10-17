package strategy

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/system"
)

/*
TestMain loads the shipped cmd/cfg/config.yml, so paper trading is tested
against the market model, quote currency and risk the run uses.
*/
func TestMain(m *testing.M) {
	viper.SetConfigFile("../cmd/cfg/config.yml")

	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "strategy tests: config not readable:", err)
		os.Exit(1)
	}

	system.Cfg = system.NewConfig()
	os.Exit(m.Run())
}
