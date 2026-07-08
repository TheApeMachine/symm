package websocket

import (
	"context"
	"math/big"
	"os"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type PaperPrivate struct {
	ctx    context.Context
	cancel context.CancelFunc
	stream *Stream
	paper  *kraken.PaperCLI
	rest   *spot.REST
}

func NewPaperPrivate(ctx context.Context) *PaperPrivate {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetViper().GetInt("system.websocket.channel.buffer")

	if buffer < 1 {
		buffer = 1
	}

	// Paper mode simulates fills, not the account. Fees are real account/market
	// data, so paper uses the same authenticated REST client as live to fetch
	// the true fee schedule — keeping paper P&L an accurate emulation of live.
	rest := spot.NewREST()
	rest.PublicKey = os.Getenv("KRAKEN_API_KEY")
	rest.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	return &PaperPrivate{
		ctx:    ctx,
		cancel: cancel,
		stream: NewStream(buffer),
		paper:  kraken.NewPaperCLI(),
		rest:   rest,
	}
}

func (private *PaperPrivate) Observe(channel string) chan []byte {
	observer := private.stream.Observe(channel)

	switch channel {
	case "balances":
		errnie.Error(private.publishBalances())
	case "orders":
		errnie.Error(private.publishOrders())
	case "executions":
		errnie.Error(private.publishExecutions())
	}

	return observer
}

func (private *PaperPrivate) TradeVolume(pairs []string) (FeeSchedule, error) {
	return fetchFeeSchedule(private.rest, pairs)
}

func (private *PaperPrivate) Submit(order *kraken.Order) error {
	response, err := private.paper.Submit(private.ctx, order)

	if err != nil {
		return err
	}

	buf, err := sonic.Marshal(response)

	if err != nil {
		return err
	}

	private.stream.Receive(buf)

	if err := private.publishBalances(); err != nil {
		return err
	}

	if err := private.publishOrders(); err != nil {
		return err
	}

	return private.publishExecutions()
}

func (private *PaperPrivate) Close() {
	private.cancel()
}

func (private *PaperPrivate) publishBalances() error {
	rows, err := private.paper.Balances(private.ctx)

	if err != nil {
		return err
	}

	buf, err := sonic.Marshal(rows)

	if err != nil {
		return err
	}

	private.stream.Receive(append([]byte(`{"channel":"balances","data":`), append(buf, '}')...))
	return nil
}

func (private *PaperPrivate) publishOrders() error {
	rows, err := private.paper.Orders(private.ctx)

	if err != nil {
		return err
	}

	buf, err := sonic.Marshal(rows)

	if err != nil {
		return err
	}

	private.stream.Receive(append([]byte(`{"channel":"orders","data":`), append(buf, '}')...))
	return nil
}

func (private *PaperPrivate) publishExecutions() error {
	rows, err := private.paper.Executions(private.ctx)

	if err != nil {
		return err
	}

	type openTrade struct {
		qty        *big.Rat
		cost       *big.Rat
		fee        *big.Rat
		priceScale int
		costScale  int
		feeScale   int
		row        kraken.ExecutionData
	}

	positions := map[string]*openTrade{}
	sort.Slice(rows, func(left int, right int) bool {
		return rows[left].Timestamp.Before(rows[right].Timestamp)
	})

	for _, row := range rows {
		if strings.TrimSpace(row.Symbol) == "" ||
			row.LastQty <= 0 ||
			row.LastPrice.Rat().Sign() <= 0 ||
			row.Cost.Rat().Sign() <= 0 ||
			row.FeeUSDEquiv.Rat().Sign() < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper executions: invalid trade row",
				nil,
			))
		}

		side := strings.ToLower(strings.TrimSpace(row.Side))

		if side != "buy" && side != "sell" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper executions: unsupported trade side",
				nil,
			))
		}

		position := positions[row.Symbol]

		if position == nil {
			position = &openTrade{
				qty:  new(big.Rat),
				cost: new(big.Rat),
				fee:  new(big.Rat),
			}
			positions[row.Symbol] = position
		}

		if scale := int(row.LastPrice.GetScale()); scale > position.priceScale {
			position.priceScale = scale
		}

		if scale := int(row.Cost.GetScale()); scale > position.costScale {
			position.costScale = scale
		}

		if scale := int(row.FeeUSDEquiv.GetScale()); scale > position.feeScale {
			position.feeScale = scale
		}

		qty := new(big.Rat).SetFloat64(row.LastQty)

		if qty == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper executions: invalid quantity",
				nil,
			))
		}

		switch side {
		case "buy":
			position.qty.Add(position.qty, qty)
			position.cost.Add(position.cost, row.Cost.Rat())
			position.fee.Add(position.fee, row.FeeUSDEquiv.Rat())
			position.row = row
		case "sell":
			if position.qty.Sign() <= 0 || qty.Cmp(position.qty) > 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"kraken paper executions: sell exceeds restored position",
					nil,
				))
			}

			ratio := new(big.Rat).Quo(qty, position.qty)
			costReduction := new(big.Rat).Mul(new(big.Rat).Set(position.cost), ratio)
			feeReduction := new(big.Rat).Mul(new(big.Rat).Set(position.fee), ratio)
			position.cost.Sub(position.cost, costReduction)
			position.fee.Sub(position.fee, feeReduction)
			position.qty.Sub(position.qty, qty)
		}
	}

	updates := kraken.ExecutionDataSlice{}

	for _, position := range positions {
		if position.qty.Sign() <= 0 || position.cost.Sign() <= 0 {
			continue
		}

		price, err := decimal.NewFromString(
			new(big.Rat).Quo(position.cost, position.qty).FloatString(position.priceScale),
		)

		if err != nil {
			return errnie.Error(err)
		}

		cost, err := decimal.NewFromString(position.cost.FloatString(position.costScale))

		if err != nil {
			return errnie.Error(err)
		}

		fee, err := decimal.NewFromString(position.fee.FloatString(position.feeScale))

		if err != nil {
			return errnie.Error(err)
		}

		qty, _ := position.qty.Float64()
		row := position.row
		row.AvgPrice = *price
		row.Cost = *cost
		row.ExecType = "snapshot"
		row.FeeUSDEquiv = *fee
		row.LastPrice = row.AvgPrice
		row.LastQty = qty
		row.OrderQty = qty
		row.OrderStatus = "filled"
		row.PositionStatus = "open"
		row.Side = "buy"
		updates = append(updates, row)
	}

	buf, err := sonic.Marshal(updates)

	if err != nil {
		return err
	}

	private.stream.Receive(append([]byte(`{"channel":"executions","data":`), append(buf, '}')...))
	return nil
}
