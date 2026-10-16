package cmd

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
storedEpochs is requested when given, else every epoch with stored ticks
(data.TickPrefix). No stored epoch is an error: there is nothing to work on.
*/
func storedEpochs(client *store.Client, requested []int64) ([]uint64, error) {
	if len(requested) > 0 {
		epochs := make([]uint64, 0, len(requested))

		for _, epoch := range requested {
			if epoch <= 0 {
				return nil, errnie.Err(errnie.Validation, fmt.Sprintf("--epoch %d is not a stored epoch (positive unix nanoseconds)", epoch), nil)
			}

			epochs = append(epochs, uint64(epoch))
		}

		return epochs, nil
	}

	epochs, err := store.Epochs(client, data.TickPrefix())

	if err != nil {
		return nil, err
	}

	if len(epochs) == 0 {
		return nil, errnie.Err(errnie.NotFound, "no stored ticks under "+data.TickPrefix(), nil)
	}

	return epochs, nil
}
