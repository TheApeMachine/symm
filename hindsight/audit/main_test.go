package audit

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/system"
)

/*
TestMain loads the shipped cmd/cfg/config.yml, so the stages are tested
against the thresholds the audit command runs with, not test-local ones.
*/
func TestMain(m *testing.M) {
	viper.SetConfigFile("../../cmd/cfg/config.yml")

	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "audit tests: config not readable:", err)
		os.Exit(1)
	}

	system.Cfg = system.NewConfig()
	system.Cfg.Audit.TakerFee = 0.004
	os.Exit(m.Run())
}
