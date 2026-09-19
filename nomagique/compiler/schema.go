package compiler

/*
Graph represents the JSON structure of a visual signal definition.
*/
type Graph struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Nodes map[string]Node `json:"nodes"`
}

/*
Node represents an atomic operation or a composed sub-graph.
*/
type Node struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Connections Connections    `json:"connections"`
	InputData   map[string]any `json:"inputData,omitempty"`
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
