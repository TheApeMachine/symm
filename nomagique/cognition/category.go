package cognition

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/* CategoryServer interprets one complete causal cut, retaining no market history. */
type CategoryServer struct {
	message *capnp.Message
	reading CategoryReading
}

func NewCategory() *CategoryServer { return &CategoryServer{} }

/* Write classifies the declared standardized coordinates exactly once each. */
func (server *CategoryServer) Write(ctx context.Context, call Category_write) error {
	server.Shutdown()
	record, err := call.Args().Cut()

	if err != nil {
		return errnie.Error(err)
	}

	if record.TypeId() != data.MetricCut_TypeID {
		return errnie.Error(errnie.Err(errnie.Validation, "category: expected a native metric cut", nil))
	}
	pointer, err := record.Value()

	if err != nil {
		return errnie.Error(err)
	}
	cut := data.MetricCut(pointer.Struct())

	if !cut.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "category: missing cut", nil))
	}

	if !cut.Complete() {
		return nil
	}
	vocabulary, err := call.Args().Vocabulary()

	if err != nil {
		return errnie.Error(err)
	}
	identities, err := call.Args().Identities()

	if err != nil {
		return errnie.Error(err)
	}
	assignments, err := call.Args().Categories()

	if err != nil {
		return errnie.Error(err)
	}

	if vocabulary.Len() == 0 || identities.Len() == 0 || assignments.Len() != identities.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "category: vocabulary and parallel evidence assignments required", nil))
	}
	metrics, err := cut.Metrics()

	if err != nil {
		return errnie.Error(err)
	}
	measured := make(map[string]data.MetricCut_Metric, metrics.Len())
	for index := range metrics.Len() {
		metric := metrics.At(index)
		identity, err := metric.Identity()

		if err != nil {
			return errnie.Error(err)
		}

		if metric.Present() && (metric.Epoch() != cut.Epoch() || metric.Sequence() > cut.Sequence()) {
			return errnie.Error(errnie.Err(errnie.Validation, "category: metric is outside the causal cut", nil))
		}
		measured[identity] = metric
	}

	if err := server.prepare(cut, vocabulary); err != nil {
		return err
	}
	transforms, err := call.Args().Transforms()
	if err != nil {
		return errnie.Error(err)
	}
	if transforms.Len() != identities.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "category: every evidence leg requires an explicit transform", nil))
	}
	return server.classify(measured, identities, assignments, transforms)
}

/* prepare owns only the pending native result message. */
func (server *CategoryServer) prepare(cut data.MetricCut, vocabulary capnp.TextList) error {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(err)
	}
	server.message = message
	server.reading, err = NewRootCategoryReading(segment)

	if err != nil {
		return errnie.Error(err)
	}
	symbol, err := cut.Symbol()

	if err != nil {
		return errnie.Error(err)
	}

	if symbol == "" || cut.Epoch() <= 0 || cut.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "category: symbol and causal stamp required", nil))
	}

	if err := server.reading.SetSymbol(symbol); err != nil {
		return errnie.Error(err)
	}
	server.reading.SetEpoch(cut.Epoch())
	server.reading.SetSequence(cut.Sequence())
	categories, err := server.reading.NewCategories(int32(vocabulary.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	seen := make(map[string]bool, vocabulary.Len())
	for index := range vocabulary.Len() {
		name, err := vocabulary.At(index)

		if err != nil {
			return errnie.Error(err)
		}

		if name == "" || seen[name] {
			return errnie.Error(errnie.Err(errnie.Validation, "category: empty or repeated vocabulary entry", nil))
		}
		seen[name] = true

		if err := categories.At(index).SetName(name); err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* classify keeps required zero legs in the conjunction and unknown evidence absent. */
func (server *CategoryServer) classify(metrics map[string]data.MetricCut_Metric, identities, assignments, transforms capnp.TextList) error {
	categories, err := server.reading.Categories()
	if err != nil {
		return errnie.Error(err)
	}
	lookup := make(map[string]int, categories.Len())
	supporting, missing := make([][]string, categories.Len()), make([][]string, categories.Len())
	logarithms := make([]float64, categories.Len())
	zero := make([]bool, categories.Len())
	for index := range categories.Len() {
		name, err := categories.At(index).Name()
		if err != nil {
			return errnie.Error(err)
		}
		lookup[name] = index
	}
	seen := make(map[[2]string]bool, identities.Len())
	for index := range identities.Len() {
		identity, err := identities.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		name, err := assignments.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		transform, err := transforms.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		position, exists := lookup[name]
		if !exists || identity == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "category: undeclared evidence assignment", nil))
		}
		key := [2]string{identity, name}
		if seen[key] {
			return errnie.Error(errnie.Err(errnie.Validation, "category: duplicate evidence assignment", nil))
		}
		seen[key] = true
		category := categories.At(position)
		category.SetAuthored(true)
		metric, exists := metrics[identity]
		if !exists || !metric.Present() {
			missing[position] = append(missing[position], identity)
			continue
		}
		affinity, err := categoryAffinity(metric.Value(), transform)
		if err != nil {
			return err
		}
		category.SetObserved(category.Observed() + 1)
		if affinity == 0 {
			zero[position] = true
			continue
		}
		logarithms[position] += math.Log(affinity)
		supporting[position] = append(supporting[position], identity)
	}
	total, defined := 0.0, 0
	for index := range categories.Len() {
		category := categories.At(index)
		category.SetDefined(category.Authored() && len(missing[index]) == 0)
		if category.Defined() {
			defined++
			if !zero[index] {
				category.SetStrength(math.Exp(logarithms[index] / float64(category.Observed())))
			}
			total += category.Strength() + 1 // LEGACY's symmetric descriptive prior.
		}
		if err := server.identities(category, supporting[index], missing[index]); err != nil {
			return err
		}
	}
	winner, entropy := -1, 0.0
	for index := range categories.Len() {
		category := categories.At(index)
		if !category.Defined() {
			continue
		}
		confidence := (category.Strength() + 1) / total
		category.SetConfidence(confidence)
		category.SetSurprisal(-math.Log2(confidence))
		entropy -= confidence * math.Log(confidence)
		if category.Strength() > 0 && (winner < 0 || category.Strength() > categories.At(winner).Strength()) {
			winner = index
		}
	}
	if defined > 1 {
		server.reading.SetUncertainty(entropy / math.Log(float64(defined)))
	}
	if winner < 0 {
		return nil
	}
	name, err := categories.At(winner).Name()
	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(server.reading.SetDominant(name))
}

/* categoryAffinity applies only the explicitly authored interpretation of a departure. */
func categoryAffinity(value float64, transform string) (float64, error) {
	switch transform {
	case "magnitude":
		return math.Abs(value), nil
	case "positive":
		if value <= 0 {
			return 0, nil
		}
		return value, nil
	case "negative":
		if value >= 0 {
			return 0, nil
		}
		return -value, nil
	default:
		return 0, errnie.Error(errnie.Err(errnie.Validation, "category: unknown evidence transform "+transform, nil))
	}
}

/* identities keeps missing inputs distinct from measured zero support. */
func (server *CategoryServer) identities(category CategoryEvidence, supporting, missing []string) error {
	support, err := category.NewSupporting(int32(len(supporting)))

	if err != nil {
		return errnie.Error(err)
	}
	absent, err := category.NewMissing(int32(len(missing)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, identity := range supporting {
		if err := support.Set(index, identity); err != nil {
			return errnie.Error(err)
		}
	}
	for index, identity := range missing {
		if err := absent.Set(index, identity); err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* Done publishes values in stable vocabulary order and releases evaluation state. */
func (server *CategoryServer) Done(ctx context.Context, call Category_done) error {
	defer server.Shutdown()
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}

	if !server.reading.IsValid() {
		result.SetIdle()
		return nil
	}
	result.SetReady()

	if err := result.Ready().SetReading(server.reading); err != nil {
		return errnie.Error(err)
	}
	categories, err := server.reading.Categories()

	if err != nil {
		return errnie.Error(err)
	}
	width := 0
	for index := range categories.Len() {
		if categories.At(index).Authored() {
			width++
		}
	}
	values, err := result.Ready().NewValues(int32(width))
	if err != nil {
		return errnie.Error(err)
	}
	supports, err := result.Ready().NewSupport(int32(width))
	if err != nil {
		return errnie.Error(err)
	}
	labels, err := result.Ready().NewLabels(int32(width))
	if err != nil {
		return errnie.Error(err)
	}
	present, err := result.Ready().NewPresent(int32(width))
	if err != nil {
		return errnie.Error(err)
	}
	position := 0
	for index := range categories.Len() {
		category := categories.At(index)
		if !category.Authored() {
			continue
		}
		values.Set(position, category.Strength())
		present.Set(position, category.Defined())
		name, err := category.Name()
		if err != nil {
			return errnie.Error(err)
		}
		if err := labels.Set(position, name); err != nil {
			return errnie.Error(err)
		}
		support, err := category.Supporting()
		if err != nil {
			return errnie.Error(err)
		}
		supports.Set(position, float64(support.Len()))
		position++
	}
	return nil
}

/* Shutdown releases only this evaluation's owned message. */
func (server *CategoryServer) Shutdown() {
	if server.message != nil {
		server.message.Release()
	}
	server.message, server.reading = nil, CategoryReading{}
}
