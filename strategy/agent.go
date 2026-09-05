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

	/*
		Skill measures the policy lane's forward competence and owns the
		execution authority that measurement justifies. Desk is the account
		the policy lane reaches once promoted; it stays nil in a learning-only
		run and no order can be produced without it.
	*/
	Skill       *SkillMeter
	Desk        ExecutionDesk
	Realization *RealizationMeter
	attribution attribution
	dispatched  uint64

	/*
		skillWindow is the end of the last forward window admitted into the
		skill estimate across all markets. Decisions issue far faster than a
		window closes, and cross-market movements are highly correlated. A
		global admission clock prevents counting overlapping windows as
		independent evidence, which would collapse measured dispersion and
		saturate confidence.
	*/
	skillWindow time.Time

	/*
		rejected counts policy intents the account did not accept, and
		lastRejection retains the most recent reason. They are reported, never
		fatal: an execution problem must not stop the agent from learning.
	*/
	rejected      uint64
	lastRejection error

	/*
		reviews carries confirmed Hindsight episodes from the delay line to the
		single workspace owner. forward is the standing account of what the
		tape offered against what the policy lane actually held.
	*/
	reviews  chan []hindsight.Episode
	reviewed map[string]struct{}
	forward  ForwardReview
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

	/*
		epochs measures this instrument's own cadence of impulse change: the
		mean interval between grid versions that actually moved. The decision
		horizon is derived from it, so a fast instrument is scored over a fast
		window and a slow one is not judged on noise.
	*/
	epochAt   time.Time
	epochMean float64
	epochs    uint64

	// exposure is the policy lane's inventory history, used to judge episodes
	// the delay line confirms after the fact.
	exposure []exposureSpan
}

/* epoch folds one observed interval between impulse changes into the mean. */
func (market *learningMarket) epoch(at time.Time) {
	if !market.epochAt.IsZero() && at.After(market.epochAt) {
		market.epochs++
		market.epochMean += (at.Sub(market.epochAt).Seconds() - market.epochMean) / float64(market.epochs)
	}

	market.epochAt = at
}

/*
horizon is the measured forward window every decision in this market is scored
over. It is unavailable until an interval has actually been observed; an
unmeasured horizon resolves nothing rather than inventing a default one.
*/
func (market *learningMarket) horizon() time.Duration {
	if market.epochs == 0 || market.epochMean <= 0 {
		return 0
	}

	return time.Duration(market.epochMean * horizonEpochs * float64(time.Second))
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
		Grid:        grid,
		Model:       learning.NewModel[[2]string, LearningAction](2048.0),
		ctx:         ctx,
		books:       books,
		pair:        pair,
		fee:         fee,
		initial:     initial.Copy(),
		Record:      record,
		markets:     make(map[string]*learningMarket), requests: make(chan learningRequest), now: time.Now,
		reviews:     make(chan []hindsight.Episode, 1), reviewed: make(map[string]struct{}),
		Skill:       NewSkillMeter(AccountNone, time.Now()),
		Realization: NewRealizationMeter(),
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
	case episodes := <-agent.reviews:
		agent.review(episodes)
	default:
	}

	return envelope
}

/* Error exposes actual failed processing to the workspace's failure boundary. */
func (agent *Agent) Error() error { return agent.err }

/* Mode reports the execution authority the measured skill currently justifies. */
func (agent *Agent) Mode() Mode {
	if agent.Skill == nil {
		return ModeLearning
	}

	mode := agent.Skill.Mode()

	if mode == ModeTrading && agent.Realization != nil && !agent.Realization.AllowsTrading() {
		return ModeLearning
	}

	return mode
}

/*
SetExecution attaches the account the agent trades once it has earned it. The
account is configuration — paper or real, the same behaviour either way — and
the agent does not earn its way between them.

Without a desk no account is attached and the agent can never leave learning,
however good its measurement looks, so a learning-only run has no code path
that reaches an account at all.
*/
func (agent *Agent) SetExecution(desk ExecutionDesk, account Account) {
	agent.Desk = desk

	if desk == nil {
		account = AccountNone
	}

	agent.Skill = NewSkillMeter(account, agent.now())
	agent.Realization = NewRealizationMeter()
}

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
