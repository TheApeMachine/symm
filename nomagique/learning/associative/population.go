package associative

import (
	"bytes"
	"context"
	"encoding/gob"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
)

/* Checkpoint supplies durable bytes; found=false explicitly denotes a first run. */
type Checkpoint interface {
	Load(context.Context) (data []byte, found bool, err error)
	Save(context.Context, []byte) error
}

/*
Population shares one grid across independent agents. Agent zero runs and learns
with the consolidated model; remaining agents explore in their own environments.
Steps serialize observations, resolution and checkpoints. Within each step,
independent agents measure concurrently, then activate concurrently after every
measurement succeeds. Environments own synchronization for any shared resources. No domain limits live here.
*/
type Population[Action comparable] struct {
	Grid       *grid.Space
	Agents     []*agent.Agent[Action]
	mutex      sync.Mutex
	checkpoint sync.Mutex
}

/* NewPopulation creates a fresh learning population with independent environments. */
func NewPopulation[Action comparable](environments ...agent.Environment[Action]) (*Population[Action], error) {
	if len(environments) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "population: at least one environment required", nil,
		))
	}
	population := &Population[Action]{Grid: grid.NewSpace()}

	for index, environment := range environments {
		member, err := agent.New(environment, model.New[string, Action](), index != 0)

		if err != nil {
			return nil, err
		}
		population.Agents = append(population.Agents, member)
	}

	return population, nil
}

/*
Step admits one observation, measures each objective and activates every agent
when that context has hot regions. History precedes current conditions. Region
authority is weighted by observed activation energy, the same weighting used
within each basin. No new market threshold or observation horizon is selected.
*/
func (population *Population[Action]) Step(observation agent.Observation) error {
	population.mutex.Lock()
	defer population.mutex.Unlock()

	if observation.At.IsZero() {
		return errnie.Error(errnie.Err(errnie.Validation, "population: observation time required", nil))
	}
	version := population.Grid.Version

	if err := population.Grid.Step(observation.Measurements); err != nil {
		return errnie.Error(err)
	}

	var measurements errgroup.Group

	for _, member := range population.Agents {
		measurements.Go(member.Measure)
	}

	if err := measurements.Wait(); err != nil {
		return errnie.Error(err)
	}

	if population.Grid.Version == version {
		return nil
	}
	label := population.Grid.UpdatedLabel
	regions, _, err := population.Grid.Regions(label)

	if err != nil {
		return errnie.Error(err)
	}

	if len(regions) == 0 {
		return nil
	}
	context := make([]uint64, 0, len(observation.History)+len(regions))
	context = append(context, observation.History...)
	strength, authority := 0.0, 0.0

	for _, region := range regions {
		context = append(context, region.Condition)
		strength += region.Strength
		authority += region.Strength * region.Authority
	}

	var activations errgroup.Group

	for _, member := range population.Agents {
		activations.Go(func() error {
			return member.Activate(label, observation.At, context, authority/strength)
		})
	}

	// Join every issued action even on failure: acceptance may be uncertain.
	return errnie.Error(activations.Wait())
}

/*
Resolve accepts only a fully developed evaluation supplied by the domain.
Every signed result trains its issuing agent. Positive explorer experience also
trains the consolidated model under the original context, action and authority.
The consolidated agent's own results are never counted twice.
*/
func (population *Population[Action]) Resolve(member int, identity uint64, value float64) error {
	population.mutex.Lock()
	defer population.mutex.Unlock()

	if member < 0 || member >= len(population.Agents) {
		return errnie.Error(errnie.Err(errnie.Validation, "population: unknown agent", nil))
	}
	decision, err := population.Agents[member].Resolve(identity, value)

	if err != nil {
		return err
	}

	if member == 0 || value <= 0 {
		return nil
	}

	return errnie.Error(population.Agents[0].Model.Observe(
		decision.Label, decision.Context, decision.Action, value, decision.Authority,
	))
}

/*
Abort releases an issued decision without training it. The domain uses this when
an outcome arrived for an action that carried no alternative: the environment
offered exactly one operation, so the result measures what happened rather than
what the choice was worth, and counting it would bury genuine evidence under
samples no decision produced.
*/
func (population *Population[Action]) Abort(member int, identity uint64) error {
	population.mutex.Lock()
	defer population.mutex.Unlock()

	if member < 0 || member >= len(population.Agents) {
		return errnie.Error(errnie.Err(errnie.Validation, "population: unknown agent", nil))
	}

	return errnie.Error(population.Agents[member].Abort(identity))
}

/*
Save encodes the grid's identity dictionary and the actual consolidated model.
Other agents, mutable environments and inflight decisions are not checkpoints.
Encoding owns the state lock; durable I/O runs after releasing it.
*/
func (population *Population[Action]) Save(ctx context.Context, checkpoint Checkpoint) error {
	population.checkpoint.Lock()
	defer population.checkpoint.Unlock()

	var data bytes.Buffer
	encoder := gob.NewEncoder(&data)

	population.mutex.Lock()
	err := encoder.Encode(population.Grid.Columns)

	if err == nil {
		// Identities and evidence form one checkpoint. A replay admission must
		// not introduce model tokens after their dictionary was encoded.
		err = population.Agents[0].Model.Encode(encoder)
	}
	population.mutex.Unlock()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "population: encode checkpoint", err))
	}

	return errnie.Error(checkpoint.Save(ctx, data.Bytes()))
}

/*
Restore loads a checkpoint into a fresh population, retaining feature identities
and cloning the learned evidence into every agent. Calling it after observations
would discard current learning and is refused. Missing data is reported explicitly.
*/
func (population *Population[Action]) Restore(ctx context.Context, checkpoint Checkpoint) (bool, error) {
	data, found, err := checkpoint.Load(ctx)

	if err != nil || !found {
		return found, errnie.Error(err)
	}
	decoder := gob.NewDecoder(bytes.NewReader(data))
	var columns [][2]string
	learned := model.New[string, Action]()

	if err := decoder.Decode(&columns); err != nil {
		return false, errnie.Error(errnie.Err(errnie.IO, "population: decode grid identities", err))
	}

	if err := decoder.Decode(learned); err != nil {
		return false, errnie.Error(errnie.Err(errnie.IO, "population: decode model", err))
	}
	restored := grid.NewSpace()

	for index, column := range columns {
		if restored.Column(column[0], column[1]) != index {
			return false, errnie.Error(errnie.Err(errnie.Validation, "population: duplicate checkpoint feature identity", nil))
		}
	}
	population.mutex.Lock()
	defer population.mutex.Unlock()

	if population.Grid.Version != 0 {
		return false, errnie.Error(errnie.Err(errnie.Conflict, "population: restore requires a fresh population", nil))
	}

	for _, member := range population.Agents {
		if member.Decisions != 0 || member.Reward.Through.Version != 0 {
			return false, errnie.Error(errnie.Err(errnie.Conflict, "population: restore requires fresh agents", nil))
		}
	}

	population.Grid = restored

	for _, member := range population.Agents {
		member.Model = learned.Clone()
	}

	return true, nil
}

/*
Learn interns a worker's named quantities before training the consolidated
model. Local column numbers have no meaning in another grid. Structural history
markers pass through unchanged; condition signs stay attached to their source
and metric name. The worker's context is never mutated.
*/
func (population *Population[Action]) Learn(
	label string, columns [][2]string, context []uint64,
	action Action, value, authority float64,
) error {
	population.mutex.Lock()
	defer population.mutex.Unlock()

	mapped := make([]uint64, len(context))

	for index, token := range context {
		if token&(uint64(1)<<63) != 0 {
			mapped[index] = token
			continue
		}

		quantity := grid.ConditionQuantity(token)

		if quantity == 0 || quantity > uint64(len(columns)) {
			return errnie.Error(errnie.Err(errnie.Validation, "population: rehearsal quantity has no identity", nil))
		}

		identity := columns[quantity-1]
		column := population.Grid.Column(identity[0], identity[1])
		mapped[index] = grid.RemapCondition(token, uint64(column+1))
	}

	return errnie.Error(population.Agents[0].Model.Observe(label, mapped, action, value, authority))
}

/*
Read scopes access to borrowed grid storage. Live observation, historical
identity admission and checkpointing all share this lock; callers must not
retain mutable grid slices beyond the callback.
*/
func (population *Population[Action]) Read(read func(*grid.Space) error) error {
	population.mutex.Lock()
	defer population.mutex.Unlock()

	return errnie.Error(read(population.Grid))
}
