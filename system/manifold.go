package system

import "github.com/spf13/viper"

type ManifoldConfig struct {
	Grid  GridConfig
	Phase Phase
}

type GridConfig struct {
	X int
	Y int
	Z int
}

type Phase struct {
	Lattice Lattice
}

type Lattice struct {
	Width int
}

func NewManifoldConfig() *ManifoldConfig {
	viper.SetDefault("manifold.grid.x", 64)
	viper.SetDefault("manifold.grid.y", 64)
	viper.SetDefault("manifold.grid.z", 64)

	return &ManifoldConfig{
		Grid: GridConfig{
			X: viper.GetInt("manifold.grid.x"),
			Y: viper.GetInt("manifold.grid.y"),
			Z: viper.GetInt("manifold.grid.z"),
		},
	}
}
