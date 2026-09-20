package compiler

import (
	"os"

	"github.com/bytedance/sonic"
)

/*
Builder reads a JSON graph definition and dynamically composes it
into a single, executable Cap'n Proto Pipeline at runtime.
*/
type Builder struct {
	graph Graph
}

func NewBuilder(jsonPath string) (*Builder, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var graph Graph
	if err := sonic.Unmarshal(data, &graph); err != nil {
		return nil, err
	}

	return &Builder{graph: graph}, nil
}

/*
Compose compiles the graph into a typed Pipeline.
*/
func (b *Builder) Compose(repos ...DefinitionRepository) (*Pipeline, error) {
	return Compile(b.graph, DefaultRegistry(), repos...)
}
