package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/workbench"
)

func main() {
	cfgFile := pflag.String("config", "", "path to config file")
	pflag.Parse()

	if err := loadConfig(*cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	system.Cfg = system.NewConfig()
	errnie.Apply(&errnie.Config{Level: viper.GetString("system.log.level")})

	warehouse := workbench.New()
	defer warehouse.Close()

	server := workbench.NewServer(warehouse)

	errnie.Info(fmt.Sprintf(
		"[workbench] listening on %s", system.Cfg.Workbench.Addr,
	))

	if err := server.Listen(); err != nil {
		errnie.Error(err)
		os.Exit(1)
	}
}

func loadConfig(cfgFile string) error {
	viper.SetConfigType("yml")

	tryRead := func(path string) error {
		viper.SetConfigFile(path)
		return viper.ReadInConfig()
	}

	if strings.TrimSpace(cfgFile) != "" {
		return tryRead(cfgFile)
	}

	paths := []string{
		"cmd/cfg/config.yml",
		"config.yml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".symm", "config.yml"))
	}

	for _, path := range paths {
		if err := tryRead(path); err == nil {
			return nil
		}
	}

	return fmt.Errorf("workbench: no config file found")
}
