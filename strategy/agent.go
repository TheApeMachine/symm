package strategy

import (
	"context"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* LearningBook is the guarded resident book shared with the live producers. */
type LearningBook interface {
	Book(string, func(*spotbook.Book))
}

/*
Agent is the single workspace owner of discovery-driven forward learning.
Four exploration lanes correspond to the requested action vocabulary; a fifth
paper lane uses the learned policy without exploratory orders. Every lane has
its own full starting wallet. No capital or inventory is shared across lanes.
*/
type Agent struct {
	Grid                       *learning.Grid
	Model                      *learning.Model[[2]string, LearningAction]
	ctx                        context.Context
	books                      LearningBook
	pair                       func(string) kraken.InstrumentPair
	fee                        func(string) *kraken.TradeVolumeFee
	initial                    *decimal.Decimal
	Record                     func(hindsight.LearningEvent) error
	markets                    map[string]*learningMarket
	requests                   chan learningRequest
	now                        func() time.Time
	err                        error
	steps, decisions, resolved uint64
}

/* learningMarket owns persistent wallets and the latest ordered impulse. */
type learningMarket struct {
	symbol, status string
	regions        []learning.Region
	sequence       []uint64
	lanes          []learningLane
	context        []uint64
	actions        []LearningAction
	events         []hindsight.LearningEvent
	at             time.Time
	gridVersion    uint64
}

/* NewAgent wires explicit numerical, execution and recording dependencies. */
func NewAgent(
	ctx context.Context,
	grid *learning.Grid,
	books LearningBook,
	pair func(string) kraken.InstrumentPair,
	fee func(string) *kraken.TradeVolumeFee,
	initial *decimal.Decimal,
	record func(hindsight.LearningEvent) error,
) (*Agent, error) {
	if grid == nil || books == nil || pair == nil || fee == nil || initial == nil || initial.Sign() <= 0 || record == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"learner: grid, book, venue rules, fees, positive capital and recorder are required",
			nil,
		))
	}

	return &Agent{
		Grid:    grid,
		Model:   learning.NewModel[[2]string, LearningAction](),
		ctx:     ctx,
		books:   books,
		pair:    pair,
		fee:     fee,
		initial: initial.Copy(),
		Record:  record,
		markets: make(map[string]*learningMarket), requests: make(chan learningRequest), now: time.Now,
	}, nil
}

/* Step consumes the current grid and settles prior actions on later book reads. */
func (agent *Agent) Step(envelope *types.Envelope) *types.Envelope {
	agent.steps++

	if envelope != nil && envelope.TypeID == types.EnvelopeLevel3 && agent.err == nil {
		agent.err = agent.advance(envelope.Level3Data)
	}

	select {
	case request := <-agent.requests:
		request.reply <- agent.view(request.symbol)
	default:
	}

	return envelope
}

/* Error exposes actual failed processing to the workspace's failure boundary. */
func (agent *Agent) Error() error { return agent.err }

/* advance uses one coherent current book for all independent virtual wallets. */
func (agent *Agent) advance(message kraken.Level3Data) error {
	market := agent.markets[message.Symbol]

	if market == nil {
		market = &learningMarket{symbol: message.Symbol, status: "waiting for executable book"}
		agent.markets[message.Symbol] = market
	}

	market.at = agent.now()
	regions, version, err := agent.Grid.Regions(message.Symbol)

	if err != nil {
		market.status = "waiting for numeric observations"
		return nil
	}

	market.regions = append(market.regions[:0], regions...)
	market.context = market.context[:0]
	for _, region := range regions {
		market.context = append(market.context, region.ID)
	}
	changed := market.gridVersion != version
	market.gridVersion = version
	market.sequence = append(market.sequence[:0], market.context...)
	market.events = market.events[:0]
	market.status = "waiting for executable book"

	agent.books.Book(message.Symbol, func(book *spotbook.Book) {
		if book == nil || book.Bids == nil || book.Asks == nil || book.Bids.High == nil || book.Asks.Low == nil {
			return
		}
		if book.Bids.High.Price.Cmp(book.Asks.Low.Price) >= 0 {
			market.status = "crossed book"
			return
		}
		if len(market.lanes) == 0 {
			err = agent.initialize(market)
		}
		if err != nil || len(market.lanes) == 0 {
			return
		}
		market.status = "learning"
		err = agent.transition(market, book, message.Timestamp, changed)
	})

	if err != nil {
		return err
	}

	for _, event := range market.events {
		if err := agent.Record(event); err != nil {
			return err
		}
	}

	return nil
}

/* initialize clones known capital and venue economics, without external flows. */
func (agent *Agent) initialize(market *learningMarket) error {
	pair, fee := agent.pair(market.symbol), agent.fee(market.symbol)

	if fee == nil || pair.Symbol == "" {
		market.status = "waiting for venue economics"
		return nil
	}
	if pair.QtyIncrement == nil || pair.QtyMin == nil || pair.CostMin == nil ||
		pair.QtyIncrement.Sign() <= 0 || pair.QtyMin.Sign() <= 0 || pair.CostMin.Sign() <= 0 ||
		fee.Fee == nil || fee.Fee.Sign() < 0 || fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return errnie.Err(errnie.Validation, "learner: invalid venue rules or fees for "+market.symbol, nil)
	}

	vocabulary := [...]types.Action{types.ActionHold, types.ActionEnter, types.ActionExit, types.ActionScale}
	market.lanes = make([]learningLane, len(vocabulary)+1)

	for index := range market.lanes {
		lane := &market.lanes[index]
		lane.paper = index == len(vocabulary)
		lane.wallet.initialize(agent.initial, pair, fee.Fee)
	}

	return nil
}
