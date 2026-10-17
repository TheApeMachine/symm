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

	for page, err := range client.listObjects(client.Context(), prefix, "/") {
		if err != nil {
			return nil, err
		}

		if page == nil {
			continue
		}

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

type keyRank struct {
	key   string
	epoch string
	tick  string
}

func sortKeys(keys []string) {
	if len(keys) < 2 {
		return
	}

	ranks := make([]keyRank, len(keys))

	for i, key := range keys {
		rest, tick, _ := cutLast(key)
		_, epoch, _ := cutLast(rest)
		ranks[i] = keyRank{key: key, epoch: epoch, tick: tick}
	}

	slices.SortFunc(ranks, func(first, second keyRank) int {
		if first.epoch != second.epoch {
			return strings.Compare(first.epoch, second.epoch)
		}

		if first.tick != second.tick {
			return strings.Compare(first.tick, second.tick)
		}

		return strings.Compare(first.key, second.key)
	})

	for i := range ranks {
		keys[i] = ranks[i].key
	}
}

func cutLast(key string) (string, string, bool) {
	idx := strings.LastIndex(key, "/")

	if idx < 0 {
		return "", key, false
	}

	return key[:idx], key[idx+1:], true
}
