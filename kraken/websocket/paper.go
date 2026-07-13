package websocket

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Paper is the simulated spot websocket and REST transport.
*/
type Paper struct {
	ctx       context.Context
	cancel    context.CancelFunc
	status    types.Status
	sync      *sync.Map
	simulator *Simulator
	lifecycle *Lifecycle
	url       string
	auth      bool
}

/*
NewPaper opens the paper spot transport.
*/
func NewPaper(
	ctx context.Context,
	simulator *Simulator,
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	paper := &Paper{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.READY,
		sync:      &sync.Map{},
		simulator: simulator,
	}

	paper.lifecycle = NewLifecycle(paper)
	return paper
}

func (paper *Paper) Initialize() error {
	paper.status = types.READY
	return nil
}

func (paper *Paper) Status() types.Status {
	return paper.simulator.Status()
}

func (paper *Paper) On(
	channel string, action func([]byte),
) {
	callbacks, ok := paper.sync.LoadOrStore(channel, []func([]byte){action})

	if ok {
		callbacks = append(callbacks.([]func([]byte)), action)
		paper.sync.Store(channel, callbacks)
	}
}

func (paper *Paper) Emit(channel string, payload json.Marshaler) error {
	raw, err := payload.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to emit payload",
			err,
		))
	}

	callbacks, ok := paper.sync.Load(channel)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to emit payload",
			nil,
		))
	}

	for _, callback := range callbacks.([]func([]byte)) {
		callback(raw)
	}

	return nil
}

func (paper *Paper) Close() {
	paper.cancel()
}

func (paper *Paper) SubBalances() error {
	return paper.lifecycle.Balance()
}

func (paper *Paper) SubExecutions(map[string]any) error {
	var model datura.Map[any]
	var err error

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("executions", "history")
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to executions",
			err,
		))
	}

	trades, ok := model["trades"].([]any)

	if !ok {
		return nil
	}

	return paper.lifecycle.Replay(trades)
}

func (paper *Paper) TradesHistory() (*kraken.TradesHistory, error) {
	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("history", "history")
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	return kraken.NewTradesHistoryFromMap(model), nil
}

func (paper *Paper) AddOrder(order *kraken.MarketOrder) error {
	command := []string{
		order.Params.Side,
		order.Params.Symbol,
		strconv.FormatFloat(order.Params.OrderQty, 'f', -1, 64),
	}

	if order.Params.OrderType == "limit" {
		command = append(
			command,
			"--type", "limit",
			"--price", strconv.FormatFloat(order.Params.LimitPrice, 'f', -1, 64),
		)
	}

	var model datura.Map[any]
	var err error

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("executions", command...)
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place paper order",
			err,
		))
	}

	return paper.lifecycle.Place(model, order.ReqID)
}

func (paper *Paper) execute(entity string, command ...string) (datura.Map[any], error) {
	input := []string{
		"paper",
	}

	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.Command("kraken", input...)

	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to "+entity,
			err,
		))
	}

	buffer := bytes.NewBuffer([]byte{})

	go func() {
		scanner := bufio.NewScanner(stdout)

		if err := scanner.Err(); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to subscribe to "+entity,
				err,
			))

			return
		}

		for scanner.Scan() {
			buffer.Write(scanner.Bytes())
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to "+entity,
			err,
		))
	}

	if err := cmd.Wait(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to "+entity+": "+stderr.String(),
			err,
		))
	}

	model := datura.Map[any]{}

	err = sonic.Unmarshal(buffer.Bytes(), &model)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to "+entity,
			err,
		))
	}

	if errCategory, ok := model["error"].(string); ok && errCategory != "" {
		message, _ := model["message"].(string)

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper: "+message,
			nil,
		))
	}

	return model, nil
}
