package system

import "github.com/spf13/viper"

type Workbench struct {
	Addr                 string
	URL                  string
	MemoryLimit          string
	MaxTempDirectorySize string
	Threads              int
}

func NewWorkbench() *Workbench {
	viper.SetDefault("workbench.addr", ":8081")
	viper.SetDefault("workbench.url", "http://127.0.0.1:8081/workbench/query")
	viper.SetDefault("workbench.memory_limit", "16GB")
	viper.SetDefault("workbench.max_temp_directory_size", "64GB")
	viper.SetDefault("workbench.threads", 4)

	return &Workbench{
		Addr:                 viper.GetString("workbench.addr"),
		URL:                  viper.GetString("workbench.url"),
		MemoryLimit:          viper.GetString("workbench.memory_limit"),
		MaxTempDirectorySize: viper.GetString("workbench.max_temp_directory_size"),
		Threads:              viper.GetInt("workbench.threads"),
	}
}
