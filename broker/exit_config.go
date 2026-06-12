package broker

import (
	"sync"

	"github.com/theapemachine/symm/config"
)

var (
	exitConfigMu     sync.Mutex
	exitConfig       config.ExitConfig
	exitConfigLoaded bool
)

func loadExitConfig() config.ExitConfig {
	exitConfigMu.Lock()
	defer exitConfigMu.Unlock()

	if exitConfigLoaded {
		return exitConfig
	}

	loaded, err := config.LoadExitConfig()

	if err == nil {
		exitConfig = loaded
		exitConfigLoaded = true
	}

	return exitConfig
}

func resetExitConfigForTest() {
	exitConfigMu.Lock()
	defer exitConfigMu.Unlock()

	exitConfig = config.ExitConfig{}
	exitConfigLoaded = false
}
