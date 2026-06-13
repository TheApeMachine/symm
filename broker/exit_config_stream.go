package broker

import (
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/symm/config"
)

/*
ExitConfigStream publishes immutable exit config snapshots for desk hot paths.
Load reads the latest snapshot without touching Viper or blocking on I/O.
*/
type ExitConfigStream struct {
	current atomic.Pointer[config.ExitConfig]
}

func NewExitConfigStream(
	initial config.ExitConfig,
) (*ExitConfigStream, error) {
	stream := &ExitConfigStream{}
	stream.store(initial)

	return stream, nil
}

func (stream *ExitConfigStream) Load() config.ExitConfig {
	if stream == nil {
		return config.ExitConfig{}
	}

	current := stream.current.Load()

	if current == nil {
		return config.ExitConfig{}
	}

	return *current
}

func (stream *ExitConfigStream) Publish(next config.ExitConfig) error {
	if stream == nil {
		return fmt.Errorf("broker: exit config stream is not initialized")
	}

	stream.store(next)

	return nil
}

func (stream *ExitConfigStream) Close() error {
	return nil
}

func (stream *ExitConfigStream) store(next config.ExitConfig) {
	snapshot := next

	stream.current.Store(&snapshot)
}
