package causal

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/causal"
)

/*
NodeStore maintains Pearl ladder node rings per symbol.
*/
type NodeStore struct {
	nodes *sync.Map
}

/*
NewNodeStore returns an empty node-ring store.
*/
func NewNodeStore() *NodeStore {
	return &NodeStore{
		nodes: &sync.Map{},
	}
}

func peekElementOK[T any](element []byte, path string) (T, bool) {
	artifact := datura.Acquire("element", datura.Artifact_Type_json)
	artifact.WithPayload(element)

	value := datura.Peek[T](artifact, path)
	artifact.Release()

	return value, true
}

func configuredNodeRing(nodeCount, capacity int) *causal.NodeRing {
	nodeRing := causal.NewNodeRing()
	configFrame, err := datura.Acquire("node-ring-config", datura.APPJSON).
		WithAttributes(datura.Map[any]{
			"config": datura.Map[any]{
				"nodeCount": float64(nodeCount),
				"capacity":  float64(capacity),
			},
		}).
		Message().Marshal()

	if err != nil {
		return nodeRing
	}

	_, _ = nodeRing.Write(configFrame)

	return nodeRing
}

/*
Observe ingests one trade update into the scoped symbol's node ring.
*/
func (nodeStore *NodeStore) Observe(symbol string, element []byte) {
	if symbol == "" || len(element) == 0 {
		return
	}

	value, _ := nodeStore.nodes.LoadOrStore(
		symbol,
		configuredNodeRing(4, viper.GetInt("signals.feed_ring_capacity")),
	)

	nodeRing := value.(*causal.NodeRing)

	price, _ := peekElementOK[float64](element, "price")
	qty, _ := peekElementOK[float64](element, "qty")
	side, _ := peekElementOK[string](element, "side")

	flow := qty

	if side == "sell" {
		flow = -qty
	}

	row := []float64{price, qty, flow, price}
	payload := make([]byte, 8*len(row))

	for index, sample := range row {
		binary.BigEndian.PutUint64(payload[index*8:], math.Float64bits(sample))
	}

	frame, err := datura.Acquire("nodes", datura.Artifact_Type_json).
		WithPayload(payload).
		Message().Marshal()

	if err != nil {
		return
	}

	nodeRing.Write(frame)
	nodeRing.Read(make([]byte, 4096))
}

/*
Nodes returns the scoped symbol's node-ring history for the Pearl ladder.
*/
func (nodeStore *NodeStore) Nodes(symbol string) *causal.NodeRing {
	ring, ok := nodeStore.nodes.Load(symbol)

	if !ok {
		return nil
	}

	return ring.(*causal.NodeRing)
}
