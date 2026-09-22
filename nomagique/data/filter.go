package data

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
FilterServer passes a structure downstream only while the value at a path
satisfies a comparison, and reports the verdict on passed. A structure that
does not satisfy it is withheld rather than replaced, so downstream nodes
observe nothing instead of observing a substitute.

A path the structure does not carry does not pass: an absent value cannot
satisfy a comparison, and treating it as one would invent evidence.
*/
type FilterServer struct {
	*runtime.System
	path      string
	operator  string
	threshold float64
	out       []byte
	passed    bool
}

func NewFilter(ctx context.Context) *FilterServer {
	server := &FilterServer{
		System:   runtime.NewSystem(ctx, "data.filter"),
		operator: "!=",
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write evaluates the comparison against the inbound structure.
*/
func (server *FilterServer) Write(ctx context.Context, call Filter_write) error {
	path, err := call.Args().Path()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read path argument",
			err,
		))
	}

	server.path = path

	operator, err := call.Args().Operator()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read operator argument",
			err,
		))
	}

	if len(operator) > 0 {
		server.operator = operator
	}

	server.threshold = call.Args().Threshold()

	if server.path == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.filter.Write] path is not defined",
			nil,
		))
	}

	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.filter.Write] failed to read data argument",
			err,
		))
	}

	server.passed = false
	server.out = nil

	if len(payload) == 0 {
		return nil
	}

	reference, err := call.Args().ReferencePath()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "filter: reference path", err))
	}

	if reference != "" || server.operator == "exists" || server.operator == "absent" {
		passed, err := server.comparePaths(payload, reference)

		if err != nil {
			return err
		}
		server.passed = passed

		if passed {
			server.out = bytes.Clone(payload)
		}
		return nil
	}
	value, found, err := readPath(payload, server.path)

	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	passed, err := compare(value, server.operator, server.threshold)

	if err != nil {
		return err
	}

	if !passed {
		return nil
	}

	server.passed = true
	server.out = bytes.Clone(payload)

	return nil
}

/*
Done emits the structure when it satisfied the comparison.
*/
func (server *FilterServer) Done(ctx context.Context, call Filter_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.filter.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetPassed(server.passed)

	if !server.passed {
		results.SetRejected()
		return nil
	}

	if err := results.SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.filter.Done] failed to set out",
			err,
		))
	}

	server.passed = false
	server.out = nil

	return nil
}

/*
compare evaluates one comparison, reporting an operator it does not implement
rather than silently admitting or rejecting the structure.
*/
func compare(value float64, operator string, threshold float64) (bool, error) {
	switch strings.TrimSpace(operator) {
	case "==":
		return value == threshold, nil
	case "!=":
		return value != threshold, nil
	case "<":
		return value < threshold, nil
	case "<=":
		return value <= threshold, nil
	case ">":
		return value > threshold, nil
	case ">=":
		return value >= threshold, nil
	}

	return false, errnie.Error(errnie.Err(
		errnie.Validation,
		"[data.filter] operator "+operator+" is not defined",
		nil,
	))
}

/* comparePaths compares numeric tuples lexicographically without rounding capture identities. */
func (server *FilterServer) comparePaths(payload []byte, reference string) (bool, error) {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	if err := decoder.Decode(&document); err != nil {
		return false, errnie.Error(errnie.Err(errnie.Validation, "filter: document", err))
	}
	paths := strings.Split(server.path, ",")
	references := strings.Split(reference, ",")
	_, found := walk(document, strings.Split(paths[0], "."))

	if server.operator == "exists" {
		return found, nil
	}

	if server.operator == "absent" {
		return !found, nil
	}

	if len(paths) != len(references) {
		return false, errnie.Error(errnie.Err(errnie.Validation, "filter: comparison tuples have different lengths", nil))
	}
	order := 0

	for index, path := range paths {
		left, found := walk(document, strings.Split(path, "."))
		right, otherFound := walk(document, strings.Split(references[index], "."))

		if !found || !otherFound {
			return false, nil
		}
		leftNumber, leftOK := left.(json.Number)
		rightNumber, rightOK := right.(json.Number)

		if !leftOK || !rightOK {
			return false, errnie.Error(errnie.Err(errnie.Validation, "filter: comparison requires numeric fields", nil))
		}
		leftRational, leftOK := new(big.Rat).SetString(string(leftNumber))
		rightRational, rightOK := new(big.Rat).SetString(string(rightNumber))

		if !leftOK || !rightOK {
			return false, errnie.Error(errnie.Err(errnie.Validation, "filter: invalid number", nil))
		}

		if order == 0 {
			order = leftRational.Cmp(rightRational)
		}
	}
	return compare(float64(order), server.operator, 0)
}
