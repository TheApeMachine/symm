package store

import (
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

func Epochs(client *Client, prefix string) ([]uint64, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[store.client] no client to list epochs under "+prefix, nil)
	}

	epochs := make([]uint64, 0)

	for page := range client.List(prefix, "/") {
		for _, common := range page.CommonPrefixes {
			if common.Prefix == nil {
				continue
			}

			epoch, err := data.KeyEpoch(prefix, *common.Prefix)

			if err != nil {
				return nil, err
			}

			epochs = append(epochs, epoch)
		}
	}

	slices.Sort(epochs)
	return epochs, nil
}

func sortKeys(keys []string) {
	order := func(key string) (string, string) {
		rest, tick, _ := cutLast(key)
		_, epoch, _ := cutLast(rest)
		return epoch, tick
	}

	slices.SortFunc(keys, func(firstKey, secondKey string) int {
		epochA, tickA := order(firstKey)
		epochB, tickB := order(secondKey)

		if epochA != epochB {
			return strings.Compare(epochA, epochB)
		}

		if tickA != tickB {
			return strings.Compare(tickA, tickB)
		}

		return strings.Compare(firstKey, secondKey)
	})
}

func cutLast(key string) (string, string, bool) {
	idx := strings.LastIndex(key, "/")

	if idx < 0 {
		return "", key, false
	}

	return key[:idx], key[idx+1:], true
}
