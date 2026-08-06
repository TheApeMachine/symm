package manifold

import (
	"fmt"
	"maps"
	"math"
	"sort"
	"time"

	"github.com/bytedance/sonic"
	mgrbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
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
	books     websocket.BookSource
	config    pfluid.Config
	domain    *pfluid.Domain
	recorder  *audit.Recorder
	tokenizer *Tokenizer
	residency int
	turnover  int
	ui        chan []byte
	binui     chan []byte
}

/*
NewSolver creates the single shared Metal domain and a spectral corpus bounded
by the same explicit event-history capacity as the live market feed.
*/
func NewSolver(
	api *websocket.API,
	ui, binui chan []byte,
	recorder *audit.Recorder,
) *Solver {
	config := pfluid.DefaultConfig()
	configuredDelta := viper.GetDuration("market.manifold.integration_interval")

	if configuredDelta > 0 && configuredDelta.Seconds() < float64(config.MaxDelta) {
		config.MaxDelta = float32(configuredDelta.Seconds())
	}

	symbols := make([]string, 0)

	if api != nil {
		api.Books().Range(func(key, _ any) bool {
			name, ok := key.(string)

			if ok {
				symbols = append(symbols, name)
			}

			return true
		})
	}

	domain, err := newDomain(config)
	errnie.Error(err)

	/*
		Residency must stay well below the lattice size in practice. The shared
		thermo-wave solve destabilizes long before one quarter-cell occupancy once
		hundreds of symbols have contributed sequentially, so the resident history
		is bounded to one thirty-second of the cells unless config overrides it.
		That keeps the newest cross-market carriers while forcing eviction to act
		before the pilot-wave gather reaches the observed non-finite regime.
	*/
	cells := config.Grid.X * config.Grid.Y * config.Grid.Z
	residency := max(cells/32, 1)

	if configuredResidency := viper.GetInt("market.manifold.residency"); configuredResidency > 0 {
		residency = configuredResidency
	}

	return &Solver{
		books:     api,
		config:    config,
		domain:    domain,
		recorder:  recorder,
		tokenizer: NewTokenizer(config, symbols),
		residency: residency,
		ui:        ui,
		binui:     binui,
	}
}

func newDomain(config pfluid.Config) (*pfluid.Domain, error) {
	var domain *pfluid.Domain

	err := compute.WithMetalInit(func() error {
		created, err := pfluid.NewDomain(config)

		if err != nil {
			return err
		}

		domain = created
		return nil
	})

	if err != nil {
		return nil, err
	}

	return domain, nil
}

/*
Update appends tokenized book samples for every changed Hawkes epoch, then
advances the shared domain once for the complete tick.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	solver.maybeRebase(thesis.At)

	attempted := false
	stepSymbol := ""
	focus := types.Focus()
	var updateErr error
	hawkesRows := utils.Measurements(thesis, types.SourceHawkes)
	symbolSet := make(map[string]struct{})

	for _, measurement := range hawkesRows {
		if measurement != nil && measurement.Symbol != "" {
			symbolSet[measurement.Symbol] = struct{}{}
		}
	}

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, name := range symbols {
		if solver.books == nil {
			break
		}

		managed := solver.books.Book(name)

		if managed == nil {
			continue
		}

		hawkes := utils.ForSymbol(hawkesRows, name)

		bidOrders := make([]*mgrbook.Order, 0)
		askOrders := make([]*mgrbook.Order, 0)

		for _, level := range managed.Bids.Levels {
			bidOrders = append(bidOrders, level.Queue()...)
		}

		for _, level := range managed.Asks.Levels {
			askOrders = append(askOrders, level.Queue()...)
		}

		buyIntensity := hawkes[len(hawkes)-1].Sample(
			types.MetricConditionalIntensity, types.SideBuy,
		).Raw
		sellIntensity := hawkes[len(hawkes)-1].Sample(
			types.MetricConditionalIntensity, types.SideSell,
		).Raw

		particles, contentIDs, err := solver.tokenizer.NewBatch(
			bidOrders,
			askOrders,
			managed.Midpoint().Float64(),
			buyIntensity,
			sellIntensity,
			name,
		)

		if err != nil {
			updateErr = errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("failed to tokenize manifold particles for %s", name),
				err,
			))

			break
		}

		if len(particles) == 0 || len(contentIDs) == 0 {
			continue
		}

		particles, contentIDs, _ = solver.filterBatch(particles, contentIDs)

		if len(particles) == 0 || len(contentIDs) == 0 {
			continue
		}

		_, err = solver.domain.Append(particles, contentIDs)

		if err != nil {
			updateErr = errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"failed to append %d manifold particles for %s: %v",
					len(particles), name, err,
				),
				err,
			))

			break
		}

		solver.turnover += len(particles)
		attempted = true

		if stepSymbol == "" || name == focus {
			stepSymbol = name
		}
	}

	if updateErr != nil {
		if attempted {
			solver.resetDomain(stepSymbol)
		}

		return updateErr
	}

	if !attempted {
		thesis.Stamp(types.SourceManifold)
		return nil
	}

	solver.evict()

	if err := solver.Step(stepSymbol, thesis.At); err != nil {
		solver.resetDomain(stepSymbol)
		return nil
	}

	thesis.Stamp(types.SourceManifold)
	return nil
}

func (solver *Solver) maybeRebase(at time.Time) {
	if solver == nil || solver.residency <= 0 || solver.turnover < solver.residency {
		return
	}

	solver.recreateDomain("turnover", at, map[string]any{
		"resident_particles": solver.domain.ParticleCount(),
		"turnover_particles": solver.turnover,
		"residency":          solver.residency,
	})
}

/*
evict bounds how much of the market stays resident in the shared field.

Every symbol appends into one domain and nothing ever leaves, so residency
grows without limit until the pilot-wave transport goes non-finite and the
manifold stops reading at all. Keeping the most recent particles holds the
field at a size it can integrate while preserving the newest state, which is
the part the strategy reads.
*/
func (solver *Solver) evict() {
	if solver.domain == nil {
		return
	}

	resident := solver.domain.ParticleCount()

	if resident <= solver.residency {
		return
	}

	/*
		Retain takes the indices to keep, and particles are appended in
		arrival order, so the tail is the newest of them.
	*/
	keep := make([]uint32, 0, solver.residency)

	for index := resident - solver.residency; index < resident; index++ {
		keep = append(keep, uint32(index))
	}

	if err := solver.domain.Retain(keep); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to bound manifold residency at %d particles",
				solver.residency,
			),
			err,
		))
	}
}

func (solver *Solver) Step(symbol string, at time.Time) error {
	_, err := solver.domain.Advance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to advance manifold for %s with %d resident particles: %v",
				symbol, solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	frame, stats, err := solver.domain.Display()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"failed to display manifold for %s with %d resident particles: %v",
				symbol, solver.domain.ParticleCount(), err,
			),
			err,
		))
	}

	if solver.binui != nil {
		payload, encodeErr := EncodeDisplay(
			symbol,
			at,
			int(stats.Width),
			int(stats.Height),
			frame,
		)

		if encodeErr != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf("failed to encode manifold display for %s", symbol),
				encodeErr,
			))
		}

		select {
		case solver.binui <- payload:
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
				maps.Copy(row, statsMap)
			}
		}

		select {
		case solver.ui <- datura.NewMap(
			"manifold", []datura.Map[any]{row},
		).MarshalAndFree():
		default:
		}
	}

	return nil
}

func (solver *Solver) filterBatch(
	particles []pfluid.Particle,
	contentIDs []uint32,
) ([]pfluid.Particle, []uint32, int) {
	if len(particles) == 0 || len(contentIDs) == 0 {
		return nil, nil, 0
	}

	keptParticles := make([]pfluid.Particle, 0, len(particles))
	keptContentIDs := make([]uint32, 0, len(contentIDs))
	dropped := 0

	for index, particle := range particles {
		if !admissibleParticle(particle, solver.config) {
			dropped++
			continue
		}

		keptParticles = append(keptParticles, particle)
		keptContentIDs = append(keptContentIDs, contentIDs[index])
	}

	return keptParticles, keptContentIDs, dropped
}

func admissibleParticle(particle pfluid.Particle, config pfluid.Config) bool {
	values := []float32{
		particle.Position.X,
		particle.Position.Y,
		particle.Position.Z,
		particle.Velocity.X,
		particle.Velocity.Y,
		particle.Velocity.Z,
		particle.Mass,
		particle.Heat,
		particle.Energy,
		particle.Phase,
		particle.Omega,
	}

	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}

	if particle.Mass <= pfluid.MinimumPilotWaveMass || particle.Heat < 0 || particle.Energy < 0 {
		return false
	}

	return particle.Omega >= config.OmegaMin && particle.Omega <= config.OmegaMax
}

func (solver *Solver) rejectBatch(
	symbol string,
	batchStart int,
	batchParticles int,
) {
	if solver == nil || solver.domain == nil || batchParticles <= 0 {
		return
	}

	resident := solver.domain.ParticleCount()

	if batchStart < 0 || batchStart >= resident {
		return
	}

	batchEnd := min(batchStart+batchParticles, resident)
	keep := make([]uint32, 0, resident-(batchEnd-batchStart))

	for index := range resident {
		if index >= batchStart && index < batchEnd {
			continue
		}

		keep = append(keep, uint32(index))
	}

	if err := solver.domain.Retain(keep); err != nil {
		solver.resetDomain(symbol)
		return
	}

	solver.turnover = max(solver.turnover-batchParticles, 0)
}

func (solver *Solver) resetDomain(symbol string) {
	solver.recreateDomain("failed_step", time.Time{}, map[string]any{
		"symbol": symbol,
	})
}

func (solver *Solver) recreateDomain(
	reason string,
	at time.Time,
	extra map[string]any,
) {
	if solver == nil {
		return
	}

	if solver.domain != nil {
		errnie.Error(solver.domain.Close())
	}

	domain, err := newDomain(solver.config)

	if err != nil {
		solver.domain = nil
		solver.turnover = 0

		message := "failed to recreate manifold domain"

		if reason == "failed_step" {
			message = fmt.Sprintf("failed to reset manifold domain after %s destabilized it", extra["symbol"])
		}

		errnie.Error(errnie.Err(
			errnie.Internal,
			message,
			err,
		))

		return
	}

	solver.domain = domain
	solver.turnover = 0

	validationAt := at

	if validationAt.IsZero() {
		validationAt = time.Now().UTC()
	}
}

/*
Close releases the one resident domain and all accumulated observations.
*/
func (solver *Solver) Close() error {
	if solver == nil {
		return nil
	}

	if solver.domain == nil {
		return nil
	}

	errnie.Error(solver.domain.Close())
	solver.domain = nil

	return nil
}
