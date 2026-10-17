package hindsight

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
annotationTrade is one print of the symbol inside the marked window.
*/
type annotationTrade struct {
	tick     uint64
	at       int64
	price    float64
	bid, ask float64
}

/*
annotationVector is the excursion a hand-placed A B C D stores. B and C are
the last trade of their tick, which is the price the tape draws. The trade
indexes count from the first trade of A, so a later read of the window
lands C on c_idx.
*/
func annotationVector(class string, marks [4]uint64, trades []annotationTrade) ([]float64, error) {
	if !slices.Contains(data.ExcursionClasses, class) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf("[hindsight] %q is not an excursion class", class), nil)
	}

	if !(marks[0] <= marks[1] && marks[1] <= marks[2] && marks[2] <= marks[3]) {
		return nil, errnie.Err(errnie.Validation, "[hindsight] A B C D must be in tape order", nil)
	}

	indexOf := func(tick uint64, last bool) (int, error) {
		found := -1

		for idx, trade := range trades {
			if trade.tick != tick {
				continue
			}

			found = idx

			if !last {
				break
			}
		}

		if found < 0 {
			return 0, errnie.Err(errnie.NotFound, fmt.Sprintf("[hindsight] tick %d has no trade to mark", tick), nil)
		}

		return found, nil
	}

	a, err := indexOf(marks[0], false)
	if err != nil {
		return nil, err
	}

	b, err := indexOf(marks[1], true)
	if err != nil {
		return nil, err
	}

	c, err := indexOf(marks[2], true)
	if err != nil {
		return nil, err
	}

	d, err := indexOf(marks[3], true)
	if err != nil {
		return nil, err
	}

	touch := func(price, side float64) float64 {
		if side > 0 {
			return side
		}

		return price
	}

	bb, cc := trades[b], trades[c]

	return []float64{
		float64(a), float64(marks[0]), float64(b), float64(marks[1]),
		float64(c), float64(marks[2]), float64(d), float64(marks[3]),
		bb.price, cc.price, touch(bb.price, bb.ask), touch(cc.price, cc.bid),
		float64(bb.at), float64(cc.at),
		float64(b), float64(marks[1]), float64(c), float64(marks[2]),
		touch(bb.price, bb.ask), touch(cc.price, cc.bid), float64(bb.at), float64(cc.at),
	}, nil
}

/*
Annotate stores one hand-placed detection for symbol in run. The four ticks
are A, B, C and D.
*/
func (service *Service) Annotate(ctx context.Context, run uint64, symbol, class string, marks [4]uint64) (string, error) {
	return service.storeMarks(ctx, data.ExcursionOrigin, run, symbol, class, marks)
}

/*
storeMarks writes the hand-placed detection under origin.
*/
func (service *Service) storeMarks(ctx context.Context, origin string, run uint64, symbol, class string, marks [4]uint64) (string, error) {
	if service == nil || service.client == nil {
		return "", errnie.Err(errnie.Validation, "[hindsight] annotate needs a store", nil)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	keys := make([]string, 0)

	for _, key := range service.client.ListKeys(data.TickPrefix(run)) {
		tick, err := tickOfKey(key)

		if err != nil {
			return "", err
		}

		if tick >= marks[0] && tick <= marks[3] {
			keys = append(keys, key)
		}
	}

	trades := make([]annotationTrade, 0)

	for value, err := range store.Download(service.client, keys) {
		if err != nil {
			return "", err
		}

		if err := ctx.Err(); err != nil {
			return "", err
		}

		ticks, err := broker.ReadTick(value)

		if err != nil {
			return "", err
		}

		index := slices.IndexFunc(ticks, func(tick *broker.SymbolTick) bool { return tick.Symbol == symbol })

		if index < 0 {
			continue
		}

		bid, ask := 0.0, 0.0

		if ticks[index].Price.Bid != nil {
			bid = ticks[index].Price.Bid.Float64()
		}

		if ticks[index].Price.Ask != nil {
			ask = ticks[index].Price.Ask.Float64()
		}

		for _, trade := range ticks[index].Trades {
			trades = append(trades, annotationTrade{
				tick:  value.Tick,
				at:    trade.Timestamp.UnixNano(),
				price: trade.Price.Float64(),
				bid:   bid,
				ask:   ask,
			})
		}
	}

	vector, err := annotationVector(class, marks, trades)

	if err != nil {
		return "", err
	}

	value := data.NewValue(origin, class, symbol)
	value.Epoch = run
	value.Tick = marks[2]
	value.FormatPrefix()
	data.AppendVector(value, vector, data.ExcursionKeys...)

	if _, err := io.Copy(service.client, value); err != nil {
		return "", err
	}

	return value.Prefix, nil
}

type annotationRequest struct {
	Run    string `json:"run"`
	Symbol string `json:"symbol"`
	Class  string `json:"class"`
	A      string `json:"a"`
	B      string `json:"b"`
	C      string `json:"c"`
	D      string `json:"d"`
}

func (service *Service) annotateRoute(ctx fiber.Ctx) error {
	var request annotationRequest

	if err := sonic.Unmarshal(ctx.Body(), &request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "annotation body is not JSON")
	}

	run, err := strconv.ParseUint(request.Run, 10, 64)

	if err != nil || request.Symbol == "" {
		return fiber.NewError(fiber.StatusBadRequest, "annotation needs a run and a symbol")
	}

	marks := [4]uint64{}

	for idx, raw := range []string{request.A, request.B, request.C, request.D} {
		marks[idx], err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "annotation ticks must be decimal strings")
		}
	}

	key, err := service.Annotate(ctx.Context(), run, request.Symbol, request.Class, marks)

	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	return ctx.JSON(struct {
		Key string `json:"key"`
	}{Key: key})
}
