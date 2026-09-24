package statistic

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
authorityWidth is the layout of one element's retained state: reading count,
summed square and summed signal power.
*/
const authorityWidth = 3

/*
listSetter is a numeric result list being filled.
*/
type listSetter interface {
	Set(int, float64)
}

/*
indexSetter is an integer result list being filled.
*/
type indexSetter interface {
	Set(int, int64)
}

/*
AuthorityServer measures how much weight each element of a list of readings
has earned, and scales each reading against the element's own history.
*/
type AuthorityServer struct {
	standard  []float64
	defined   []bool
	authority []float64
	energy    []float64
	index     []int64
	state     []float64
}

func NewAuthority() *AuthorityServer {
	return &AuthorityServer{}
}

func (server *AuthorityServer) Write(ctx context.Context, call Authority_write) error {
	args := call.Args()
	value, err := args.Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.authority: failed to read value", err))
	}

	defined, err := args.Defined()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.authority: failed to read defined", err))
	}

	prior, err := args.Prior()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.authority: failed to read prior", err))
	}

	known, err := args.Known()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "statistic.authority: failed to read known", err))
	}

	if defined.Len() != value.Len() || prior.Len() != known.Len()*authorityWidth {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"statistic.authority: %d values with %d flags, %d prior numbers for %d records",
				value.Len(), defined.Len(), prior.Len(), known.Len(),
			),
			nil,
		))
	}

	count := value.Len()
	records := make([]float64, count*authorityWidth)

	for element := range min(count, known.Len()) {
		if !known.At(element) {
			continue
		}

		for offset := range authorityWidth {
			records[element*authorityWidth+offset] = prior.At(element*authorityWidth + offset)
		}
	}

	server.standard = make([]float64, count)
	server.defined = make([]bool, count)
	server.authority = make([]float64, count)
	server.energy = make([]float64, count)
	server.index = server.index[:0]
	server.state = server.state[:0]

	for element := range count {
		if !defined.At(element) {
			continue
		}

		server.observe(records[element*authorityWidth:(element+1)*authorityWidth], element, value.At(element))
	}

	mature := 0.0

	for element := range count {
		mature = math.Max(mature, records[element*authorityWidth])
	}

	if mature == 0 {
		return nil
	}

	for element := range count {
		server.authority[element] = records[element*authorityWidth+2] / mature
		server.energy[element] = server.standard[element] * server.standard[element] * server.authority[element]
	}

	return nil
}

/*
observe scales one reading against the element's history before it, then
adds the reading to that history.
*/
func (server *AuthorityServer) observe(record []float64, element int, reading float64) {
	if record[0] > 0 && record[1] > 0 {
		standard := reading * math.Sqrt(record[0]/record[1])
		power := standard * standard
		server.standard[element] = standard
		server.defined[element] = true
		record[2] += power / (1 + power)
	}

	record[0]++
	record[1] += reading * reading
	server.index = append(server.index, int64(element))
	server.state = append(server.state, record...)
}

func (server *AuthorityServer) Done(ctx context.Context, call Authority_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.authority: failed to allocate results", err))
	}

	lists := []struct {
		values []float64
		alloc  func(int32) (listSetter, error)
	}{
		{server.standard, func(size int32) (listSetter, error) { return results.NewStandard(size) }},
		{server.authority, func(size int32) (listSetter, error) { return results.NewAuthority(size) }},
		{server.energy, func(size int32) (listSetter, error) { return results.NewEnergy(size) }},
		{server.state, func(size int32) (listSetter, error) { return results.NewState(size) }},
	}

	for _, list := range lists {
		allocated, err := list.alloc(int32(len(list.values)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "statistic.authority: failed to allocate a list", err))
		}

		for position, number := range list.values {
			allocated.Set(position, number)
		}
	}

	defined, err := results.NewDefined(int32(len(server.defined)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.authority: failed to allocate defined", err))
	}

	for element, flag := range server.defined {
		defined.Set(element, flag)
	}

	index, err := results.NewIndex(int32(len(server.index)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "statistic.authority: failed to allocate index", err))
	}

	for position, element := range server.index {
		index.Set(position, element)
	}

	server.standard, server.defined, server.authority, server.energy = nil, nil, nil, nil
	server.index, server.state = server.index[:0], server.state[:0]
	return nil
}
