package types

import "time"

/*
GraphNodeWire carries one measurement node for UI serialization without exposing
Gonum internals on the websocket frame.
*/
type GraphNodeWire struct {
	Key         string      `json:"key"`
	Measurement Measurement `json:"measurement"`
}

/*
GraphEdgeWire carries one typed relationship between measurement node keys.
*/
type GraphEdgeWire struct {
	From         string    `json:"from"`
	To           string    `json:"to"`
	Type         EdgeType  `json:"type"`
	At           time.Time `json:"at"`
	ObservedFrom time.Time `json:"observedFrom"`
}

/*
GraphFrame is the wire snapshot of one symbol-local evidence graph.
*/
type GraphFrame struct {
	Symbol string          `json:"symbol"`
	At     time.Time       `json:"at"`
	Nodes  []GraphNodeWire `json:"nodes"`
	Edges  []GraphEdgeWire `json:"edges"`
}

/*
Frame serializes the current Gonum topology into a UI-safe wire snapshot.
*/
func (evidenceGraph *Graph) Frame() GraphFrame {
	frame := GraphFrame{
		Symbol: evidenceGraph.Symbol,
		At:     evidenceGraph.At,
		Nodes:  make([]GraphNodeWire, 0),
		Edges:  make([]GraphEdgeWire, 0),
	}

	for _, node := range evidenceGraph.Nodes() {
		frame.Nodes = append(frame.Nodes, GraphNodeWire{
			Key:         node.Key,
			Measurement: node.Measurement,
		})
	}

	for _, edge := range evidenceGraph.Edges() {
		frame.Edges = append(frame.Edges, GraphEdgeWire{
			From: edge.From, To: edge.To, Type: edge.Type,
			At: edge.At, ObservedFrom: edge.ObservedFrom,
		})
	}

	return frame
}
