package statistic

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/internal/dependence"
	"github.com/theapemachine/symm/nomagique/store"
)

/* LagSearchServer evaluates a single immutable pair, retaining no market state. */
type LagSearchServer struct {
	values  [14]float64
	present [14]bool
}

func NewLagSearch() *LagSearchServer { return &LagSearchServer{} }

/* Write searches the exact observation-supported LEGACY lead/lag profile. */
func (server *LagSearchServer) Write(ctx context.Context, call LagSearch_write) error {
	*server = LagSearchServer{}
	left, err := readPricePath(call.Args().Left)

	if err != nil {
		return err
	}
	right, err := readPricePath(call.Args().Right)

	if err != nil {
		return err
	}
	count := min(len(left.Points), len(right.Points))

	if count <= 2 {
		return nil
	}
	spacing := int64(min(left.Spacing, right.Spacing))

	if spacing <= 0 {
		return nil
	}
	span := count - 2

	if spacing > math.MaxInt64/int64(span) || left.Points[0].At < math.MinInt64+spacing*int64(span) || left.Points[len(left.Points)-1].At > math.MaxInt64-spacing*int64(span) {
		return errnie.Error(errnie.Err(errnie.Validation, "lag search: timestamp offset overflows int64", nil))
	}
	profile := make([]dependence.Estimate, span*2+1)
	selected, candidates := -1, 0
	for index := range profile {
		lag := index - span
		profile[index] = left.Compare(&right, int64(lag)*spacing)

		if lag == 0 || !profile[index].Defined {
			continue
		}
		candidates++

		if selected < 0 || math.Abs(profile[index].Correlation) > math.Abs(profile[selected].Correlation) {
			selected = index
		}
	}

	if selected < 0 {
		return nil
	}
	best, zero := profile[selected], profile[span]
	lag := selected - span
	seconds := float64(spacing) / 1e9
	server.values = [14]float64{zero.Correlation, best.Correlation, float64(lag), float64(lag) * seconds,
		math.Abs(best.Correlation) - math.Abs(zero.Correlation), math.Abs(float64(lag)) / float64(span), float64(span), float64(candidates),
		0, 0, best.Support, float64(len(left.Returns)), float64(len(right.Returns)), math.Sqrt(2 * math.Log(float64(candidates+1)) / float64(count-1))}
	for index := range server.present {
		server.present[index] = index != 8 && index != 9
	}
	server.present[0], server.present[4] = zero.Defined, zero.Defined

	if selected == 0 || selected == len(profile)-1 || !profile[selected-1].Defined || !profile[selected+1].Defined {
		return nil
	}
	difference := 2*math.Abs(best.Correlation) - math.Abs(profile[selected-1].Correlation) - math.Abs(profile[selected+1].Correlation)
	server.values[8], server.values[9] = difference/2, difference/(seconds*seconds)
	server.present[8], server.present[9] = true, true
	return nil
}

/* Done emits independently defined fields without manufacturing empty history. */
func (server *LagSearchServer) Done(ctx context.Context, call LagSearch_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	values, err := result.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(err)
	}
	present, err := result.NewPresent(int32(len(server.present)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range server.values {
		values.Set(index, value)
		present.Set(index, server.present[index])
	}
	*server = LagSearchServer{}
	return nil
}

/* readPricePath validates real ordered prices before interval mathematics. */
func readPricePath(read func() (store.PricePoint_List, error)) (dependence.Path, error) {
	source, err := read()

	if err != nil {
		return dependence.Path{}, errnie.Error(err)
	}
	path := dependence.Path{Points: make([]dependence.Point, source.Len())}
	for index := range source.Len() {
		value := source.At(index)

		if value.Value() <= 0 || index > 0 && value.At() <= source.At(index-1).At() {
			return dependence.Path{}, errnie.Error(errnie.Err(errnie.Validation, "lag search: positive ordered price points required", nil))
		}
		path.Points[index] = dependence.Point{At: value.At(), Value: value.Value()}
	}
	path.Measure()
	return path, nil
}
