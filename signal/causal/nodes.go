package causal

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	feed "github.com/theapemachine/symm/signal"
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

/*
Observe ingests one trade update into the scoped symbol's node ring.
*/
func (nodeStore *NodeStore) Observe(symbol string, element []byte) {
	if symbol == "" || len(element) == 0 {
		return
	}

	value, _ := nodeStore.nodes.LoadOrStore(
		symbol, algorithm.NewNodeRing(4, viper.GetInt("signals.feed_ring_capacity")),
	)

	nodeRing := value.(*algorithm.NodeRing)

	price, _ := feed.PeekElementOK[float64](element, "price")
	qty, _ := feed.PeekElementOK[float64](element, "qty")
	side, _ := feed.PeekElementOK[string](element, "side")

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
func (nodeStore *NodeStore) Nodes(symbol string) *algorithm.NodeRing {
	ring, ok := nodeStore.nodes.Load(symbol)

	if !ok {
		return nil
	}

	return ring.(*algorithm.NodeRing)
}
