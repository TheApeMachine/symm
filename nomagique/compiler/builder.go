package compiler

import (
	"encoding/json"
	"os"

	"github.com/theapemachine/symm/nomagique"
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
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}

	return &Builder{graph: graph}, nil
}

/*
Interests extracts the required data keys from the graph's source nodes.
*/
func (b *Builder) Interests() [][]string {
	var interests [][]string
	for _, node := range b.graph.Nodes {
		if node.Type == "source" || node.Type == "data.Source" || node.ID == "source" || node.ID == "src" {
			if cfg, ok := node.InputData["_config"].(map[string]any); ok {
				if interestStr, ok := cfg["interests"].(string); ok {
					// Temporary fallback mapping for legacy single-string configs
					switch interestStr {
					case "trade":
						interests = append(interests, []string{"trade", "price"})
					case "ticker":
						interests = append(interests, []string{"ticker", "data", "last"})
					case "level3":
						interests = append(interests, []string{"level3", "price"})
					default:
						interests = append(interests, []string{interestStr})
					}
				} else if interestArr, ok := cfg["interests"].([]any); ok {
					// Handle the new structure where interests is an array of arrays
					for _, arr := range interestArr {
						if path, ok := arr.([]any); ok {
							var strPath []string
							for _, p := range path {
								if s, ok := p.(string); ok {
									strPath = append(strPath, s)
								}
							}
							if len(strPath) > 0 {
								interests = append(interests, strPath)
							}
						}
					}
				}
			}
		}
	}
	return interests
}

/*
Compose dynamically wires the graph at runtime into a nomagique.Number pipeline.
It delegates to Compile using the DefaultRegistry.
*/
func (b *Builder) Compose() (nomagique.Number[any], error) {
	compiled, err := Compile[any](b.graph, DefaultRegistry())
	if err != nil {
		return nil, err
	}

	return nomagique.Number[any](compiled), nil
}
