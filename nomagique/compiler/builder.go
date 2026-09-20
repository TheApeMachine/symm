package compiler

import (
	"os"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Builder reads a JSON graph definition and dynamically composes it
into a single, executable `Value` pipeline at runtime.
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
Compose dynamically wires the graph at runtime into a nomagique.Number pipeline.
It delegates to Compile using the DefaultRegistry.
*/
func (b *Builder) Compose(repos ...DefinitionRepository) (types.StreamNode[any, any], error) {
	compiled, err := Compile[any, any](b.graph, DefaultRegistry(), repos...)
	if err != nil {
		return nil, err
	}

	return types.StreamNode[any, any](compiled), nil
}
