package compiler

import (
	"os"
)

/*
Builder reads a JSON graph definition and dynamically composes it
into a single, executable Cap'n Proto Program at runtime.
*/
type Builder struct {
	graph Graph
}

func NewBuilder(jsonPath string) (*Builder, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	graph, err := ParseGraph(data)
	if err != nil {
		return nil, err
	}

	return &Builder{graph: graph}, nil
}

/*
Compose compiles the graph into an immutable typed Program.
*/
func (b *Builder) Compose(repos ...DefinitionRepository) (*Program, error) {
	return Compile(b.graph, DefaultRegistry(), repos...)
}
