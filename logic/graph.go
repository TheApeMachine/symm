package logic

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
EdgeType names how two measurement nodes relate within one symbol epoch.
*/
type EdgeType string

const (
	Supports     EdgeType = "supports"
	Contradicts  EdgeType = "contradicts"
	Conditions   EdgeType = "conditions"
	Leads        EdgeType = "leads"
	Lags         EdgeType = "lags"
	Redundant    EdgeType = "redundant"
	Independent  EdgeType = "independent"
	Stale        EdgeType = "stale"
	Incomparable EdgeType = "incomparable"
)

/*
Node is one typed measurement reference retained for graph provenance.
*/
type Node struct {
	Key         string
	Measurement types.Measurement
}

/*
Edge records a directed relationship with the evaluation time and the evidence
interval that justified it.
*/
type Edge struct {
	Type         EdgeType
	From         string
	To           string
	At           time.Time
	ObservedFrom time.Time
}

/*
Graph holds measurement nodes and their relationships for one symbol epoch.
*/
type Graph struct {
	Symbol string
	At     time.Time
	Nodes  []Node
	Edges  []Edge
}

/*
NewGraph starts an empty relationship graph for one symbol.
*/
func NewGraph(symbol string) *Graph {
	return &Graph{
		Symbol: symbol,
		Nodes:  []Node{},
		Edges:  []Edge{},
	}
}

/*
AddNode retains one measurement when it belongs to this graph's symbol.
*/
func (graph *Graph) AddNode(measurement *types.Measurement) bool {
	if measurement == nil || measurement.Symbol != graph.Symbol {
		return false
	}

	key := measurementKey(measurement)

	for _, node := range graph.Nodes {
		if node.Key == key {
			return false
		}
	}

	graph.Nodes = append(graph.Nodes, Node{
		Key:         key,
		Measurement: *measurement,
	})

	if graph.At.IsZero() || measurement.At.After(graph.At) {
		graph.At = measurement.At
	}

	return true
}

/*
Relate appends one directed edge between existing node keys.
*/
func (graph *Graph) Relate(
	fromKey string,
	toKey string,
	edgeType EdgeType,
	at time.Time,
	observedFrom time.Time,
) bool {
	if fromKey == "" || toKey == "" || edgeType == "" {
		return false
	}

	if !graph.hasNode(fromKey) || !graph.hasNode(toKey) {
		return false
	}

	graph.Edges = append(graph.Edges, Edge{
		Type:         edgeType,
		From:         fromKey,
		To:           toKey,
		At:           at,
		ObservedFrom: observedFrom,
	})

	if graph.At.IsZero() || at.After(graph.At) {
		graph.At = at
	}

	return true
}

func (graph *Graph) hasNode(key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}

	return false
}

func measurementKey(measurement *types.Measurement) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		measurement.Stream,
		measurement.Metric,
		measurement.Subject,
		measurement.Side,
		measurement.Symbol,
	)
}
