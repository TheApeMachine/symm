package compiler

import "encoding/json"

/*
Graph represents the JSON structure of a visual signal definition.
*/
type Graph struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Nodes map[string]Node `json:"nodes"`
	// Origins names, for every node a definition brought in, the definition
	// and the id it has there, so a surface drawing that definition can find
	// its own node again after expansion prefixed it.
	Origins map[string]Origin `json:"-"`
}

/*
Origin is where an expanded node was authored.
*/
type Origin struct {
	Definition string
	Node       string
}

/*
Node represents an atomic operation or a composed sub-graph.
Zero any fields exist on Node.
*/
type Node struct {
	ID          string                     `json:"id"`
	Type        string                     `json:"type"`
	Connections Connections                `json:"connections"`
	InputData   map[string]json.RawMessage `json:"inputData,omitempty"`
}

/*
Connections define the routing of data between nodes.
*/
type Connections struct {
	Inputs  map[string][]ConnectionTarget `json:"inputs"`
	Outputs map[string][]ConnectionTarget `json:"outputs"`
}

/*
ConnectionTarget specifies the downstream or upstream endpoint.
*/
type ConnectionTarget struct {
	NodeID   string `json:"nodeId"`
	PortName string `json:"portName"`
}

/*
DefinitionRepository resolves definition references (such as "definition:<name>")
into child Graphs for recursive in-memory compilation.
*/
type DefinitionRepository interface {
	Load(name string) (Graph, error)
}
