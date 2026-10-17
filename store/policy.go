package store

import (
	"fmt"
	"iter"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

func PolicyGates(client *Client) ([]data.PolicyGate, error) {
	return policyGates(
		func(prefix string) ([]uint64, error) { return Epochs(client, prefix) },
		func(prefixes ...string) iter.Seq2[*data.Value, error] { return Values(client, prefixes...) },
	)
}

func policyGates(
	epochs func(string) ([]uint64, error),
	values func(...string) iter.Seq2[*data.Value, error],
) ([]data.PolicyGate, error) {
	all := make([]uint64, 0)

	for _, prefix := range data.PolicyPrefixes() {
		found, err := epochs(prefix)

		if err != nil {
			return nil, errnie.Err(errnie.IO, "store: listing policy epochs under "+prefix, err)
		}

		all = append(all, found...)
	}

	slices.Sort(all)
	gates := make([]data.PolicyGate, 0)

	for _, epoch := range slices.Compact(all) {
		var gate *data.PolicyGate

		for value, err := range values(data.PolicyPrefixes(epoch)...) {
			if err != nil {
				return nil, err
			}

			read, err := data.ReadPolicyGate(value)

			if err != nil {
				return nil, err
			}

			gate = &read
			break
		}

		if gate == nil {
			return nil, errnie.Err(errnie.NotFound, fmt.Sprintf("store: policy epoch %d lists no readable policy", epoch), nil)
		}

		gates = append(gates, *gate)
	}

	return gates, nil
}
