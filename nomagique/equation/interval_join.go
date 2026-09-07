package equation

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
IntervalJoin emits overlapping pairs from two ordered, internally disjoint
interval paths. Input is one record with left/right Primitive collections of
(from, to] int64 intervals. Touching endpoints do not overlap. It advances the
interval ending first, visiting O(left + right) intervals instead of constructing
a Cartesian product. Emitted pairs retain the original immutable carriers.
*/
type IntervalJoin struct {
	core.PrimitiveError
	paths    [2][]core.Primitive
	bounds   [2][][2]int64
	position [2]int
	open     bool
	current  core.Primitive
}

func NewIntervalJoin() *IntervalJoin { return &IntervalJoin{} }

func (join *IntervalJoin) Next(input core.Primitive) core.Primitive {
	if !join.open {
		if err := join.load(input); err != nil {
			join.Error(err)
			return nil
		}

		join.open = true
	}

	for join.position[0] < len(join.paths[0]) && join.position[1] < len(join.paths[1]) {
		left, right := join.position[0], join.position[1]
		leftBounds, rightBounds := join.bounds[0][left], join.bounds[1][right]

		if leftBounds[1] <= rightBounds[1] {
			join.position[0]++
		}

		if rightBounds[1] <= leftBounds[1] {
			join.position[1]++
		}

		if leftBounds[0] < rightBounds[1] && rightBounds[0] < leftBounds[1] {
			join.current = core.From([]core.Primitive{join.paths[0][left], join.paths[1][right]})
			return join.current
		}
	}

	join.paths = [2][]core.Primitive{}
	join.position = [2]int{}
	join.open = false
	return nil
}

func (join *IntervalJoin) Read() any { return core.To[any](join.current) }

func (join *IntervalJoin) load(input core.Primitive) error {
	count := 0
	record := core.Yield(
		transport.NewIO(core.From(map[string]core.Primitive{})), input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			count++
			return fields
		},
	)

	if err := record.Error(); err != nil {
		return err
	}

	if count != 1 {
		return fmt.Errorf("%w: interval join requires one path record, received %d", core.ErrShape, count)
	}

	fields := core.To[map[string]core.Primitive](record)

	for side, name := range []string{"left", "right"} {
		path, err := core.Field[[]core.Primitive](fields, name)

		if err != nil {
			return err
		}

		if err := join.readPath(side, path); err != nil {
			return err
		}
	}

	return nil
}

func (join *IntervalJoin) readPath(side int, path []core.Primitive) error {
	join.paths[side] = path
	join.bounds[side] = join.bounds[side][:0]

	for index, interval := range path {
		fields := core.To[map[string]core.Primitive](interval)

		if err := interval.Error(); err != nil {
			return err
		}

		from, err := core.Field[int64](fields, "from")

		if err != nil {
			return err
		}

		to, err := core.Field[int64](fields, "to")

		if err != nil {
			return err
		}

		if from >= to || (index > 0 && from < join.bounds[side][index-1][1]) {
			return fmt.Errorf("%w: interval join path %d interval %d must be positive and ordered without internal overlap", core.ErrShape, side, index)
		}

		join.bounds[side] = append(join.bounds[side], [2]int64{from, to})
	}

	return nil
}
