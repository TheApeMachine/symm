package logic

import (
	"embed"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"go.yaml.in/yaml/v3"
)

//go:embed rules/tree.yml
var embedded embed.FS

var playbookThresholdConfig config.ThresholdConfig

type Tree struct {
	Branches        []*Branch `yaml:"branches" json:"branches"`
	thresholdConfig config.ThresholdConfig
	entryTransitTTL time.Duration
}

/*
LoadTree decodes the embedded playbook without publishing.
*/
func LoadTree() (*Tree, error) {
	reader, err := embedded.Open("rules/tree.yml")

	if err != nil {
		return nil, err
	}

	defer reader.Close()

	tree := &Tree{}

	if err := yaml.NewDecoder(reader).Decode(tree); errnie.Error(err) != nil {
		return tree, err
	}

	thresholdConfig, err := config.LoadThresholdConfig()

	if errnie.Error(err) != nil {
		return nil, err
	}

	tree.thresholdConfig = thresholdConfig
	tree.entryTransitTTL = viper.GetDuration("trading.entry.transit_ttl")
	playbookThresholdConfig = thresholdConfig
	applyConfigThresholds(tree, thresholdConfig)

	return tree, nil
}

func (tree *Tree) ThresholdConfig() config.ThresholdConfig {
	if tree == nil {
		return config.ThresholdConfig{}
	}

	return tree.thresholdConfig
}

/*
NewTree decodes the embedded playbook and publishes it to the ui bus.
*/
func NewTree(bus *internal.Bus) (*Tree, error) {
	tree, err := LoadTree()

	if errnie.Error(err) != nil {
		return nil, err
	}

	if bus == nil {
		return tree, nil
	}

	if err := PublishTree(bus, tree); errnie.Error(err) != nil {
		return nil, err
	}

	return tree, nil
}

/*
PublishTree sends the playbook branches to the dashboard websocket path.
*/
func PublishTree(bus *internal.Bus, tree *Tree) error {
	if bus == nil {
		return errnie.Error(errnie.Require(map[string]any{
			"bus": bus,
		}))
	}

	if tree == nil {
		return errnie.Error(errnie.Require(map[string]any{
			"tree": tree,
		}))
	}

	return bus.Send(internal.ChannelUI, "decision_tree", tree)
}

func (tree *Tree) Evaluate(measurements []Measurement, holdings *Holdings) (*Evaluation, error) {
	for _, branch := range tree.Branches {
		evaluation, err := branch.Evaluate(measurements, holdings)

		if errnie.Error(err) != nil {
			return nil, errnie.Error(err)
		}

		if evaluation != nil {
			return evaluation, nil
		}
	}

	return nil, nil
}
