package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/mat"
	"math"
)

/*
	Snapshot copies retained parameters and unresolved predictions through the

existing native checkpoint capability. It is outside observation processing.
*/
func (server *ResonanceServer) Snapshot(ctx context.Context, call runtime.Snapshot_snapshot) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	if server.manifold == nil {
		return errnie.Error(result.SetData(nil))
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	state, err := NewRootResonanceState(segment)
	if err != nil {
		return errnie.Error(err)
	}
	if err := server.save(state); err != nil {
		return err
	}
	encoded, err := message.Marshal()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetData(encoded))
}

/* Restore validates an entire candidate before replacing the retained owner. */
func (server *ResonanceServer) Restore(ctx context.Context, call runtime.Snapshot_restore) error {
	encoded, err := call.Args().Data()
	if err != nil {
		return errnie.Error(err)
	}
	if len(encoded) == 0 {
		*server = ResonanceServer{}
		return nil
	}
	message, err := capnp.Unmarshal(encoded)
	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	state, err := ReadRootResonanceState(message)
	if err != nil {
		return errnie.Error(err)
	}
	candidate := NewResonance()
	if err := candidate.load(state); err != nil {
		return err
	}
	*server = *candidate
	return nil
}

func (server *ResonanceServer) save(state ResonanceState) error {
	labels, err := state.NewFeatureIdentities(int32(len(server.identities)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, identity := range server.identities {
		if err := labels.Set(index, identity); err != nil {
			return errnie.Error(err)
		}
	}
	state.SetEpoch(server.epoch)
	state.SetSequence(server.sequence)
	state.SetObservations(int64(server.observations))
	state.SetResolved(int64(server.resolved))
	state.SetReference(server.reference)
	state.SetReturnCount(int64(server.returnCount))
	state.SetReturnMean(server.returnMean)
	state.SetReturnM2(server.returnM2)
	state.SetWidth(uint32(server.manifold.arch[0]))
	for _, field := range []struct {
		values   []*mat.Dense
		allocate func(int32) (ResonanceMatrix_List, error)
	}{{server.manifold.generativeWeights, state.NewGenerative}, {server.manifold.recognitionWeights, state.NewRecognition}, {server.manifold.temporalOperators, state.NewTemporal}} {
		list, err := field.allocate(int32(len(field.values)))
		if err != nil {
			return errnie.Error(err)
		}
		for index, value := range field.values {
			rows, columns := value.Dims()
			if err := writeResonanceMatrix(list.At(index), rows, columns, value.RawMatrix().Data); err != nil {
				return err
			}
		}
	}
	for _, field := range []struct {
		values   []*mat.VecDense
		allocate func(int32) (ResonanceVector_List, error)
	}{{server.manifold.latentStates, state.NewLatents}, {server.manifold.workspace.prevLatents, state.NewPrevious}} {
		list, err := field.allocate(int32(len(field.values)))
		if err != nil {
			return errnie.Error(err)
		}
		for index, value := range field.values {
			if err := writeFloats(value.RawVector().Data, list.At(index).NewValues); err != nil {
				return err
			}
		}
	}
	heads, err := state.NewHeads(int32(len(server.manifold.taskLearners)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, task := range server.manifold.taskLearners {
		head := heads.At(index)
		head.SetRank(int64(task.rank))
		head.SetObservations(int64(task.observations))
		head.SetResidual(task.residual)
		head.SetSupport(int64(server.manifold.taskSupport[index]))
		head.SetModelLoss(server.manifold.taskModelLoss.AtVec(index))
		head.SetBaselineLoss(server.manifold.taskBaselineLoss.AtVec(index))
		if err := writeFloats(task.beta, head.NewBeta); err != nil {
			return err
		}
		for _, field := range []struct {
			values   [][]float64
			allocate func() (ResonanceMatrix, error)
		}{{task.inverse, head.NewInverse}, {task.null, head.NewNullSpace}} {
			matrix, err := field.allocate()
			if err != nil {
				return errnie.Error(err)
			}
			flat := make([]float64, 0, len(task.beta)*len(task.beta))
			for _, row := range field.values {
				flat = append(flat, row...)
			}
			if err := writeResonanceMatrix(matrix, len(task.beta), len(task.beta), flat); err != nil {
				return err
			}
		}
	}
	pending, err := state.NewPending(int32(len(server.pending)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, item := range server.pending {
		row := pending.At(index)
		row.SetReference(item.reference)
		row.SetIssued(int64(item.issued))
		row.SetNoise(item.noise)
		if err := writeFloats(item.features, row.NewFeatures); err != nil {
			return err
		}
		if err := writeForecast(item.predictions, row.NewPredictions); err != nil {
			return err
		}
	}
	memory, err := state.NewMemory()
	if err != nil {
		return errnie.Error(err)
	}
	memory.SetCount(int64(server.memory.count))
	memory.SetLeftEnergy(server.memory.leftEnergy)
	memory.SetRightEnergy(server.memory.rightEnergy)
	memory.SetCovariance(server.memory.covariance)
	for _, field := range []struct {
		values   []float64
		allocate func(int32) (capnp.Float64List, error)
	}{{server.memory.previous, memory.NewPrevious}, {server.memory.left, memory.NewLeft}, {server.memory.right, memory.NewRight}} {
		if err := writeFloats(field.values, field.allocate); err != nil {
			return err
		}
	}
	return nil
}

func (server *ResonanceServer) load(state ResonanceState) error {
	width := int(state.Width())
	heads, err := state.Heads()
	if err != nil {
		return errnie.Error(err)
	}
	if width == 0 || heads.Len() == 0 || state.Observations() <= 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: dimensions and observations required", nil))
	}
	labels, err := state.FeatureIdentities()
	if err != nil {
		return errnie.Error(err)
	}
	if err := server.identify(labels, width); err != nil {
		return err
	}
	server.epoch = state.Epoch()
	server.sequence = state.Sequence()
	server.observations = int(state.Observations())
	server.resolved = int(state.Resolved())
	server.reference = state.Reference()
	server.returnCount = int(state.ReturnCount())
	server.returnMean = state.ReturnMean()
	server.returnM2 = state.ReturnM2()
	server.alpha = 1 / float64(server.observations)
	server.manifold = newResonanceManifold([]int{width, width, width}, heads.Len(), server.alpha, readoutLatents)
	for _, field := range []struct {
		values []*mat.Dense
		read   func() (ResonanceMatrix_List, error)
	}{{server.manifold.generativeWeights, state.Generative}, {server.manifold.recognitionWeights, state.Recognition}, {server.manifold.temporalOperators, state.Temporal}} {
		list, err := field.read()
		if err != nil {
			return errnie.Error(err)
		}
		if list.Len() != len(field.values) {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: matrix family width changed", nil))
		}
		for index, value := range field.values {
			rows, columns := value.Dims()
			if err := readResonanceMatrix(list.At(index), rows, columns, value.RawMatrix().Data); err != nil {
				return err
			}
		}
	}
	for _, field := range []struct {
		values []*mat.VecDense
		read   func() (ResonanceVector_List, error)
	}{{server.manifold.latentStates, state.Latents}, {server.manifold.workspace.prevLatents, state.Previous}} {
		list, err := field.read()
		if err != nil {
			return errnie.Error(err)
		}
		if list.Len() != len(field.values) {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: vector family width changed", nil))
		}
		for index, value := range field.values {
			values, err := floatsOf(list.At(index).Values())
			if err != nil {
				return err
			}
			if len(values) != value.Len() {
				return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: latent width changed", nil))
			}
			copy(value.RawVector().Data, values)
		}
	}
	server.manifold.temporalPriorsReady = true
	for index := range heads.Len() {
		head := heads.At(index)
		task := server.manifold.taskLearners[index]
		beta, err := floatsOf(head.Beta())
		if err != nil {
			return err
		}
		if len(beta) != len(task.beta) {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: affine width changed", nil))
		}
		copy(task.beta, beta)
		task.rank = int(head.Rank())
		task.observations = int(head.Observations())
		task.residual = head.Residual()
		for _, field := range []struct {
			values [][]float64
			read   func() (ResonanceMatrix, error)
		}{{task.inverse, head.Inverse}, {task.null, head.NullSpace}} {
			matrix, err := field.read()
			if err != nil {
				return errnie.Error(err)
			}
			flat := make([]float64, len(beta)*len(beta))
			if err := readResonanceMatrix(matrix, len(beta), len(beta), flat); err != nil {
				return err
			}
			for row := range field.values {
				copy(field.values[row], flat[row*len(beta):(row+1)*len(beta)])
			}
		}
		copy(server.manifold.taskWeights.RawRowView(index), beta[1:])
		server.manifold.taskBias.SetVec(index, beta[0])
		server.manifold.taskSupport[index] = int(head.Support())
		server.manifold.taskModelLoss.SetVec(index, head.ModelLoss())
		server.manifold.taskBaselineLoss.SetVec(index, head.BaselineLoss())
		if head.ModelLoss() > 0 {
			server.manifold.taskVar.SetVec(index, head.ModelLoss())
			server.manifold.taskScale.SetVec(index, math.Sqrt(head.ModelLoss()))
			server.manifold.taskPrecision.SetVec(index, 1/head.ModelLoss())
			server.manifold.taskSkill.SetVec(index, head.BaselineLoss()/head.ModelLoss())
			server.manifold.taskScaleReady[index] = true
			server.manifold.taskSkillReady[index] = true
		}
	}
	pending, err := state.Pending()
	if err != nil {
		return errnie.Error(err)
	}
	for index := range pending.Len() {
		row := pending.At(index)
		features, err := floatsOf(row.Features())
		if err != nil {
			return err
		}
		forecasts, err := row.Predictions()
		if err != nil {
			return errnie.Error(err)
		}
		item := &resonanceReference{reference: row.Reference(), features: features, issued: int(row.Issued()), noise: row.Noise(), predictions: make([]resonanceForecast, forecasts.Len())}
		if len(features) != server.manifold.readoutDim || forecasts.Len() > heads.Len() || item.issued > server.observations {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: invalid pending causal row", nil))
		}
		for horizon := range forecasts.Len() {
			value := forecasts.At(horizon)
			item.predictions[horizon] = resonanceForecast{Value: value.Value(), Scale: value.Scale(), DegreesOfFreedom: value.DegreesOfFreedom(), Ready: value.Ready(), Innovation: value.Innovation(), Reset: value.Reset()}
		}
		server.pending = append(server.pending, item)
	}
	memory, err := state.Memory()
	if err != nil {
		return errnie.Error(err)
	}
	server.memory.count = int(memory.Count())
	server.memory.leftEnergy = memory.LeftEnergy()
	server.memory.rightEnergy = memory.RightEnergy()
	server.memory.covariance = memory.Covariance()
	for _, field := range []struct {
		values *[]float64
		read   func() (capnp.Float64List, error)
	}{{&server.memory.previous, memory.Previous}, {&server.memory.left, memory.Left}, {&server.memory.right, memory.Right}} {
		values, err := floatsOf(field.read())
		if err != nil {
			return err
		}
		if len(values) != width {
			return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: memory width changed", nil))
		}
		*field.values = values
	}
	return nil
}

func writeResonanceMatrix(target ResonanceMatrix, rows, columns int, values []float64) error {
	target.SetRows(uint32(rows))
	target.SetColumns(uint32(columns))
	return writeFloats(values, target.NewValues)
}
func readResonanceMatrix(source ResonanceMatrix, rows, columns int, target []float64) error {
	values, err := source.Values()
	if err != nil {
		return errnie.Error(err)
	}
	if int(source.Rows()) != rows || int(source.Columns()) != columns || values.Len() != len(target) {
		return errnie.Error(errnie.Err(errnie.Validation, "resonance snapshot: matrix shape changed", nil))
	}
	for index := range target {
		target[index] = values.At(index)
	}
	return nil
}
