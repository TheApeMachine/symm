package system

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type Runtime struct {
	Workspace *Workspace
}

type Workspace struct {
	Buffer uint32
	Mask   int64
}

func NewRuntime() *Runtime {
	viper.SetDefault("runtime.workspace.buffer", 4096)
	buffer := uint32(viper.GetInt("runtime.workspace.buffer"))

	runtime := &Runtime{
		Workspace: &Workspace{
			Buffer: uint32(viper.GetInt("runtime.workspace.buffer")),
			Mask:   int64(buffer - 1),
		},
	}

	if buffer <= 0 || (buffer&(buffer-1)) != 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[config|runtime] buffer capacity must be a power of 2",
			nil,
		))
	}

	return runtime
}
