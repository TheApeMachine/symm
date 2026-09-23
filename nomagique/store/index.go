package store

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* indexed is one retained document and its decoded order coordinate. */
type indexed struct {
	document []byte
	order    []*big.Rat
}

/* IndexServer owns the partitioned, ordered documents appended to it. */
type IndexServer struct {
	*runtime.System
	partitions map[string][]indexed
	answers    [][]byte
}

func NewIndex(ctx context.Context) *IndexServer {
	return &IndexServer{
		System:     runtime.NewSystem(ctx, "store.index"),
		partitions: make(map[string][]indexed),
	}
}

func (server *IndexServer) Write(ctx context.Context, call Index_write) error {
	args := call.Args()
	server.answers = nil
	partition, err := args.Partition()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "index: partition path", err))
	}
	orderText, err := args.Order()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "index: order paths", err))
	}

	if partition == "" || orderText == "" {
		return server.Error(errnie.Err(errnie.Validation, "index: partition and order paths are required", nil))
	}
	order := strings.Split(orderText, ",")
	appends, err := args.Append()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "index: appends", err))
	}

	partitions := strings.Split(partition, ",")

	if err := arrived(appends, func(document []byte) error {
		return server.append(document, partitions, order)
	}); err != nil {
		return server.Error(err)
	}
	requests, err := args.Request()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "index: requests", err))
	}

	// Answers line up with request slots: a slot nothing asked on stays empty.
	for slot := range requests.Len() {
		request, err := requests.At(slot)

		if err != nil {
			return server.Error(errnie.Err(errnie.Validation, "index: request", err))
		}
		var answer []byte

		if len(request) > 0 {
			answer, err = server.answer(request, order)

			if err != nil {
				return server.Error(err)
			}
		}
		server.answers = append(server.answers, answer)
	}
	return nil
}

func (server *IndexServer) append(document []byte, partitions []string, order []string) error {
	decoded, err := decode(document)

	if err != nil {
		return err
	}
	values := make([]any, len(partitions))

	for index, path := range partitions {
		values[index], _ = at(decoded, strings.TrimSpace(path))
	}
	name, err := partitionName(values)

	if err != nil {
		return err
	}
	coordinate, err := coordinates(decoded, order)

	if err != nil {
		return err
	}
	list := server.partitions[name]

	if len(list) > 0 && compare(coordinate, list[len(list)-1].order) < 0 {
		return errnie.Err(errnie.Validation, "index: document appended out of order in partition "+name, nil)
	}
	server.partitions[name] = append(list, indexed{document: bytes.Clone(document), order: coordinate})
	return nil
}

/* answer resolves one request against its partition. */
func (server *IndexServer) answer(raw []byte, order []string) ([]byte, error) {
	var request struct {
		Partition []any                      `json:"partition"`
		Position  *int                       `json:"position"`
		Seek      string                     `json:"seek"`
		From      json.RawMessage            `json:"from"`
		Where     map[string]json.RawMessage `json:"where"`
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(&request); err != nil {
		return nil, errnie.Err(errnie.Validation, "index: request", err)
	}
	name, err := partitionName(request.Partition)

	if err != nil {
		return nil, err
	}
	list := server.partitions[name]
	position := -1

	switch {
	case request.Position != nil:
		position = *request.Position

		if position < 0 || position >= len(list) {
			position = -1
		}
	case request.Seek == "first" || request.Seek == "last":
		from, err := decode(request.From)

		if err != nil {
			return nil, err
		}
		target, err := coordinates(from, order)

		if err != nil {
			return nil, err
		}
		position, err = seek(list, target, request.Seek, request.Where)

		if err != nil {
			return nil, err
		}
	default:
		return nil, errnie.Err(errnie.Validation, "index: a request needs a position or a first/last seek", nil)
	}
	answer := map[string]any{"found": position >= 0, "count": len(list)}

	if position >= 0 {
		answer["position"] = position
		answer["document"] = json.RawMessage(list[position].document)
	}
	encoded, err := json.Marshal(answer)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "index: encode answer", err)
	}
	return encoded, nil
}

/* partitionName is the canonical encoding of a partition's values. */
func partitionName(values []any) (string, error) {
	encoded, err := json.Marshal(values)

	if err != nil {
		return "", errnie.Err(errnie.Validation, "index: partition values", err)
	}
	return string(encoded), nil
}

func seek(list []indexed, target []*big.Rat, direction string, where map[string]json.RawMessage) (int, error) {
	if direction == "first" {
		position := sort.Search(len(list), func(index int) bool { return compare(list[index].order, target) >= 0 })

		if position == len(list) {
			return -1, nil
		}
		return position, nil
	}

	for position := sort.Search(len(list), func(index int) bool { return compare(list[index].order, target) > 0 }) - 1; position >= 0; position-- {
		matched, err := matches(list[position].document, where)

		if err != nil || matched {
			return position, err
		}
	}
	return -1, nil
}

func matches(document []byte, where map[string]json.RawMessage) (bool, error) {
	if len(where) == 0 {
		return true, nil
	}
	decoded, err := decode(document)

	if err != nil {
		return false, err
	}

	for path, raw := range where {
		expected, err := decode(raw)

		if err != nil {
			return false, err
		}
		value, found := at(decoded, path)

		if !found || !reflect.DeepEqual(value, expected) {
			return false, nil
		}
	}
	return true, nil
}

func coordinates(document any, order []string) ([]*big.Rat, error) {
	coordinate := make([]*big.Rat, len(order))

	for index, path := range order {
		value, found := at(document, strings.TrimSpace(path))
		number, numeric := value.(json.Number)

		if !found || !numeric {
			return nil, errnie.Err(errnie.Validation, "index: order path "+path+" is not a number", nil)
		}
		exact, ok := new(big.Rat).SetString(number.String())

		if !ok {
			return nil, errnie.Err(errnie.Validation, "index: order path "+path+" is not a number", nil)
		}
		coordinate[index] = exact
	}
	return coordinate, nil
}

func compare(left, right []*big.Rat) int {
	for index := range left {
		if order := left[index].Cmp(right[index]); order != 0 {
			return order
		}
	}
	return 0
}

func decode(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any

	if err := decoder.Decode(&value); err != nil {
		return nil, errnie.Err(errnie.Validation, "index: document is not JSON", err)
	}
	return value, nil
}

func at(document any, path string) (any, bool) {
	current := document

	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)

		if !ok {
			return nil, false
		}
		current, ok = object[segment]

		if !ok {
			return nil, false
		}
	}
	return current, true
}

/* arrived hands every nonempty gathered arrival to apply, in slot order. */
func arrived(list capnp.DataList, apply func([]byte) error) error {
	for slot := range list.Len() {
		value, err := list.At(slot)

		if err != nil {
			return errnie.Err(errnie.Validation, "index: arrival", err)
		}

		if len(value) > 0 {
			if err := apply(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (server *IndexServer) Done(ctx context.Context, call Index_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "index: allocate result", err))
	}
	answers, err := results.NewAnswers(int32(len(server.answers)))

	if err != nil {
		return server.Error(errnie.Err(errnie.Internal, "index: allocate answers", err))
	}

	for position, answer := range server.answers {
		if err := answers.Set(position, answer); err != nil {
			return server.Error(errnie.Err(errnie.Internal, "index: emit answer", err))
		}
	}
	server.answers = nil
	return nil
}
