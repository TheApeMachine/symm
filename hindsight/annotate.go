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
	"github.com/theapemachine/symm/data"
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
markedTrades reads one symbol's prints between two ticks from the tape already
in memory.
*/
func markedTrades(tape *runTape, symbol string, from, to uint64) []annotationTrade {
	if tape == nil {
		return nil
	}

	trades := make([]annotationTrade, 0)

	for _, row := range tape.bySymbol[symbol] {
		if row.frame == nil || row.tick == nil || row.frame.Tick < from || row.frame.Tick > to {
			continue
		}

		bid, ask := 0.0, 0.0

		if row.tick.Price.Bid != nil {
			bid = row.tick.Price.Bid.Float64()
		}

		if row.tick.Price.Ask != nil {
			ask = row.tick.Price.Ask.Float64()
		}

		for _, trade := range row.tick.Trades {
			trades = append(trades, annotationTrade{
				tick:  row.frame.Tick,
				at:    trade.Timestamp.UnixNano(),
				price: trade.Price.Float64(),
				bid:   bid,
				ask:   ask,
			})
		}
	}

	return trades
}

/*
writeMarks stores the hand-placed detection. The prints come from the caller.
*/
func (service *Service) writeMarks(origin string, run uint64, symbol, class string, marks [4]uint64, trades []annotationTrade) (string, error) {
	if service == nil || service.client == nil {
		return "", errnie.Err(errnie.Validation, "[hindsight] annotate needs a store", nil)
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

/*
storeMarks writes the hand-placed detection under origin from the run tape
already loaded. It does not list or download the run again.
*/
func (service *Service) storeMarks(ctx context.Context, origin string, run uint64, symbol, class string, marks [4]uint64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return "", err
	}

	return service.writeMarks(origin, run, symbol, class, marks, markedTrades(loaded, symbol, marks[0], marks[3]))
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
