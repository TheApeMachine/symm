package broker

import (
	"sync"

	"github.com/theapemachine/symm/config"
)

var (
	exitConfigMu sync.Mutex
	exitConfig   config.ExitConfig
)

func loadExitConfig() config.ExitConfig {
	exitConfigMu.Lock()
	defer exitConfigMu.Unlock()

	if exitConfig.TrailDefault > 0 || exitConfig.SpreadScale > 0 || exitConfig.StopFloor > 0 {
		return exitConfig
	}

	loaded, err := config.LoadExitConfig()

	if err == nil {
		exitConfig = loaded
	}

	return exitConfig
}

func resetExitConfigForTest() {
	exitConfigMu.Lock()
	defer exitConfigMu.Unlock()

	exitConfig = config.ExitConfig{}
}
