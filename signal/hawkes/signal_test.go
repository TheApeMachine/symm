package hawkes

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func newInstrumentedSignal(testingTB testing.TB, tree *dmt.Tree) (*Signal, *algorithm.Excitation) {
	if testingTB != nil {
		testingTB.Helper()
	}

	ctx, cancel := context.WithCancel(context.Background())
	excitation := algorithm.NewExcitation()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        newTestPool(testingTB),
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			excitation,
			probability.NewClassifier(
				datura.Acquire("hawkes-classifier", datura.APPJSON).Poke(
					[]string{"frenzy", "saturation", "organic", "exhaustion"},
					"inputs",
				),
			),
		),
	}

	return signal, excitation
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertTradeExcitation(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}
