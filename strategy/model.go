package strategy

import (
	"context"
	"strings"
	"sync"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System `json:"-"`
	desk            *broker.Desk
	store           *store.Client
	sequencers      *sync.Map
	path            *Path
	policy          *iradix.Tree[*data.Value]
	explore         *iradix.Tree[*data.Value]
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	client *store.Client,
) *Model {
	if client == nil {
		client = store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)
	}

	model := &Model{
		System:     runtime.NewSystem(ctx, "model"),
		desk:       desk,
		store:      client,
		sequencers: &sync.Map{},
		path:       NewPath(),
		policy:     iradix.New[*data.Value](),
		explore:    iradix.New[*data.Value](),
	}

	if client != nil {
		model.loadPolicyFromStore()
		model.loadExploreFromStore()
	}

	return model
}

func (model *Model) Evaluate(
	out *data.Value,
	observed []*nmstore.Region,
) *data.Value {
	symbol := out.Scope
	enter := !model.desk.Has(symbol, true)

	untypedSequencer, ok := model.sequencers.Load(symbol)

	if !ok {
		return out
	}

	sequencer := untypedSequencer.(*tokenizer.Sequencer)

	out.Append(observed, "lit")
	step := sequencer.Step(observed, enter)
	cut := step.Token
	out.Append(cut, "cut")

	pathKey := model.path.After(model.path.PolicyLive(symbol, enter), cut)

	if pathKey != "" {
		out.Append(pathKey, "path")
	}

	if cut.String() != "" {
		if enter {
			model.path.AppendTrigger(symbol, cut)
		} else {
			model.path.Append(symbol, cut)
		}
	}

	disagree := false
	values := make(map[string]*data.Value)

	for _, tree := range []*iradix.Tree[*data.Value]{model.policy, model.explore} {
		tree.Root().WalkPrefix([]byte(pathKey), func(k []byte, v *data.Value) bool {
			var stored string
			v.Lookup(&stored, "action")

			switch stored {
			case "enter":
				if enter {
					disagree = true
					return false
				}

				values[string(k)] = v
			case "exit":
				if !enter {
					disagree = true
					return false
				}

				values[string(k)] = v
			}

			return true
		})
	}

	if !disagree {
		if enter {
			model.desk.Enter(symbol, pathKey, true, true)
			model.path.Reset(symbol)
			return out
		}

		position := model.desk.Position(symbol, true)
		model.desk.Exit(symbol, pathKey, true)

		for key, value := range values {
			if position.PnL.Sign() > 0 {
				value.Append(0.05, "threshold")
			} else {
				value.Append(-0.05, "threshold")
			}

			model.store.WriteAtPrefix(
				"model/explore/"+key+"/enter.bin",
				value.Bytes(),
			)

			model.explore.Insert(
				[]byte(key),
				value,
			)

			threshold := 0.0

			for idx, index := range value.Index {
				if strings.Contains(index, "threshold") {
					var thresh float64
					value.At(&thresh, idx)
					threshold += thresh
				}
			}

			if threshold > 0.8 {
				model.store.DeletePrefix("model/explore/" + key + "/")
				model.explore.DeletePrefix([]byte(key))
			}

			if threshold < 0.2 {
				model.store.WriteAtPrefix(
					"model/policy/"+key+"/enter.bin",
					value.Bytes(),
				)

				model.policy.Insert(
					[]byte(key),
					value,
				)

				model.store.DeletePrefix("model/explore/" + key + "/")
				model.explore.DeletePrefix([]byte(key))
			}
		}

		model.writePaths(position)
	}

	model.explorePath(symbol, pathKey, enter)
	model.path.Reset(symbol)
	return out
}

func (model *Model) explorePath(symbol string, pathKey string, enter bool) {
	if enter {
		position := model.desk.Enter(symbol, pathKey, true, false)

		model.explore.Insert(
			[]byte(pathKey),
			data.NewValue(
				"model", "explore", symbol,
			).Append(
				"enter", "action",
			).Append(
				position, "position",
			),
		)
	}
}

func (model *Model) writePaths(position *broker.Position) {
	if position == nil || model == nil || model.store == nil {
		return
	}

	if position.EntryPath == "" && position.ExitPath == "" {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[model.writePaths] missing entry and exit path", nil,
		))

		return
	}

	if position.EntryPath != "" {
		model.exploreInsertAction(position.EntryPath, "enter")
		model.store.WriteAtPrefix(
			"model/explore/"+position.EntryPath+"/enter.bin",
			data.NewValue(
				"model", "explore", position.Symbol,
			).Append(
				"enter", "action",
			).Bytes(),
		)
	}

	if position.ExitPath != "" {
		model.exploreInsertAction(position.ExitPath, "exit")
		model.store.WriteAtPrefix(
			"model/explore/"+position.ExitPath+"/exit.bin",
			data.NewValue(
				"model", "explore", position.Symbol,
			).Append(
				"exit", "action",
			).Bytes(),
		)
	}

	model.path.Reset(position.Symbol)
}
