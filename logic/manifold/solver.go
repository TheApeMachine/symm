package manifold

import (
	"sync"
	"time"

	"github.com/bytedance/sonic"
	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/signal/compute"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver owns one resident Sensorium domain for the complete market universe.
Symbols contribute observations to the same gas and wave fields; they are not
split into independent simulations that cannot interfere.
*/
type Solver struct {
	mu        sync.Mutex
	config    pfluid.Config
	domain    *pfluid.Domain
	recorder  *audit.Recorder
	tokenizer *Tokenizer
	ui        chan []byte
	binui     chan []byte
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(ui, binui chan []byte, recorder *audit.Recorder) *Solver {
	config := pfluid.DefaultConfig()
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 && configuredDelta.Seconds() < float64(config.MaxDelta) {
		config.MaxDelta = float32(configuredDelta.Seconds())
	}

	var domain *pfluid.Domain

	errnie.Error(compute.WithMetalInit(func() error {
		created, err := pfluid.NewDomain(config)

		if err != nil {
			return err
		}

		domain = created
		return nil
	}))

	return &Solver{
		config:    config,
		domain:    domain,
		recorder:  recorder,
		tokenizer: NewTokenizer(config, nil),
		ui:        ui,
		binui:     binui,
	}
}

/*
Update appends tokenized book samples for every changed Hawkes epoch, then
always advances the shared domain once for this tick and publishes symbol views
of that physical state. Inject is Hawkes-gated; the step is not.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	solver.mu.Lock()
	defer solver.mu.Unlock()

	if thesis.BookManager == nil {
		return nil
	}

	bidOrders := make([]*mgrbook.Order, 0)
	askOrders := make([]*mgrbook.Order, 0)

	for _, bookName := range thesis.BookManager.GetBooks() {
		book := thesis.BookManager.GetBook(bookName)
		hawkes := utils.ForSymbol(utils.Measurements(thesis, types.SourceHawkes), bookName)

		if len(hawkes) == 0 {
			continue
		}

		for _, level := range book.Bids.Levels {
			bidOrders = append(bidOrders, level.Queue()...)
		}

		for _, level := range book.Asks.Levels {
			askOrders = append(askOrders, level.Queue()...)
		}

		particles, contentIDs := solver.tokenizer.NewBatch(
			bidOrders,
			askOrders,
			book.Midpoint().Float64(),
			hawkes[len(hawkes)-1].Sample(
				types.MetricConditionalIntensity, types.SideBuy,
			).Raw,
			hawkes[len(hawkes)-1].Sample(
				types.MetricConditionalIntensity, types.SideSell,
			).Raw,
			bookName,
		)

		if len(particles) == 0 || len(contentIDs) == 0 {
			continue
		}

		solver.domain.Append(particles, contentIDs)
		errnie.Error(solver.Step(bookName, thesis.At))
	}

	return nil
}

func (solver *Solver) Step(symbol string, at time.Time) error {
	diagnostics, err := solver.domain.Advance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to advance domain",
			err,
		))
	}

	if solver.recorder != nil {
		solver.recorder.Write(diagnostics)
	}

	frame, stats, err := solver.domain.Display()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to display domain",
			err,
		))
	}

	if solver.binui != nil {
		select {
		case solver.binui <- frame:
		default:
		}
	}

	if solver.ui != nil {
		row := datura.NewMap(
			"source", "manifold",
			"symbol", symbol,
			"at", at.Format(time.RFC3339),
		)

		if statsBytes, err := sonic.Marshal(stats); err == nil {
			var statsMap map[string]any
			if err := sonic.Unmarshal(statsBytes, &statsMap); err == nil {
				for k, v := range statsMap {
					row[k] = v
				}
			}
		}

		select {
		case solver.ui <- datura.NewMap("manifold", []datura.Map[any]{row}).MarshalAndFree():
		default:
		}
	}

	return nil
}

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	if solver.domain == nil {
		return nil
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil

	return nil
}
