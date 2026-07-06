package broker

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

const (
	channelTicker     = "ticker"
	channelBalances   = "balances"
	channelExecutions = "executions"
)

type Desk struct {
	ctx        context.Context
	cancel     context.CancelFunc
	channels   map[string]chan []byte
	public     websocket.Socket
	private    websocket.Private
	balance    *kraken.BalanceDataSlice
	executions []*kraken.ExecutionDataSlice
	positions  *sync.Map
	UIForward  chan []byte
}

func NewDesk(
	ctx context.Context,
	public websocket.Socket,
	private websocket.Private,
) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	if public == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: public stream required",
			nil,
		))
	}

	if private == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: private stream required",
			nil,
		))
	}

	return &Desk{
		ctx:       ctx,
		cancel:    cancel,
		public:    public,
		private:   private,
		positions: &sync.Map{},
		channels: map[string]chan []byte{
			channelTicker:     public.Observe(channelTicker),
			channelBalances:   private.Observe(channelBalances),
			channelExecutions: private.Observe(channelExecutions),
		},
	}, nil
}

func (desk *Desk) Ready() bool {
	return errnie.Require(map[string]any{
		"balance": desk.balance,
	}) == nil
}

func (desk *Desk) OpenPositions() int {
	open := 0

	desk.positions.Range(func(_ any, value any) bool {
		open += len(value.([]*Position))
		return true
	})

	return open
}

/*
Run processes websocket and private frame streams until ctx closes.
*/
func (desk *Desk) Run() (err error) {
	for {
		select {
		case <-desk.ctx.Done():
			return nil
		case msg := <-desk.channels[channelBalances]:
			desk.balance = kraken.NewBalanceDataSlice(msg)
		case msg := <-desk.channels[channelExecutions]:
			slice := kraken.NewExecutionDataSlice(msg)
			desk.executions = append(desk.executions, slice)
		case msg := <-desk.channels[channelTicker]:
			for _, ticker := range kraken.NewTickerDataSlice(msg) {
				position, ok := desk.positions.Load(ticker.Symbol)

				if ok {
					for _, position := range position.([]*Position) {
						position.Update(ticker)
					}
				}
			}
		}

		positions := make([]PositionData, 0)

		desk.positions.Range(func(_ any, value any) bool {
			for _, position := range value.([]*Position) {
				positions = append(positions, position.Data())
			}

			return true
		})

		out := datura.Map[any]{}

		if desk.balance != nil {
			out["balance"] = desk.balance
		}

		if len(desk.executions) > 0 {
			out["executions"] = desk.executions
		}

		if len(positions) > 0 {
			out["positions"] = positions
		}

		desk.UIForward <- out.Marshal()
	}
}

func (desk *Desk) Buy(symbol string, fraction float64, price float64) error {
	position, err := NewPosition(
		desk.private,
		desk.balance,
		symbol,
		fraction,
		price,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if err := position.Enter(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	positionData := position.Data()
	positions, ok := desk.positions.LoadOrStore(positionData.Symbol, []*Position{position})

	if ok {
		desk.positions.Store(
			positionData.Symbol,
			append(positions.([]*Position), position),
		)
	}

	return nil
}

func (desk *Desk) Sell(symbol string) (err error) {
	symbol = strings.TrimSpace(symbol)
	positions, ok := desk.positions.Load(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found",
			nil,
		))
	}

	for _, position := range positions.([]*Position) {
		err = errors.Join(err, position.Exit())
	}

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	desk.positions.Delete(symbol)
	return nil
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
