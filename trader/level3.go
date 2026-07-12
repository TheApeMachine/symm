package trader

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

type level3Analyzer interface {
	ObserveLevel3(kraken.Level3Data, int, int, manifold.Level3Book) manifold.ProcessResult
	AdvanceLevel3(string)
}

type level3Instrument interface {
	Pair(string) (kraken.InstrumentPair, error)
	RefreshLevel3(string) error
}

type level3Book interface {
	manifold.Level3Book
	Reset(string)
}

/*
Level3 is the single owner of ordered trade/L3 toxicity state, the trusted L3
book, manifold observation, and fair per-symbol advancement scheduling.
*/
type Level3 struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     atomic.Value
	observed   atomic.Uint64
	stopped    atomic.Bool
	signals    []types.Signal[any]
	ring       *structure.MPMCRing[level3Frame]
	frames     *Level3Frames
	mailbox    *MeasurementMailbox
	scheduler  *Level3Scheduler
	wake       chan struct{}
	uiHub      chan []byte
	instrument level3Instrument
	analyzer   level3Analyzer
	book       level3Book
	recovering map[string]manifold.InvalidReason
}

/*
NewLevel3 constructs the bounded lock-free ingress and fixed measurement
mailbox before starting their sole owner goroutine.
*/
func NewLevel3(
	ctx context.Context,
	signal *Signal,
	uiHub chan []byte,
	instrument level3Instrument,
	analyzer level3Analyzer,
	book level3Book,
) *Level3 {
	if ctx == nil || signal == nil || instrument == nil || analyzer == nil || book == nil {
		return nil
	}

	mailbox, err := newLevel3Mailbox(signal)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "failed to create level3 mailbox", err))
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	ring, err := structure.NewMPMCRing[level3Frame](
		ctx,
		viper.GetInt("market.l3_ring_capacity"),
	)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "failed to create level3 ring", err))
		cancel()
		return nil
	}

	level3 := &Level3{
		ctx:        ctx,
		cancel:     cancel,
		signals:    signal.Level3,
		ring:       ring,
		frames:     NewLevel3Frames(),
		mailbox:    mailbox,
		scheduler:  NewLevel3Scheduler(),
		wake:       make(chan struct{}, 1),
		uiHub:      uiHub,
		instrument: instrument,
		analyzer:   analyzer,
		book:       book,
		recovering: map[string]manifold.InvalidReason{},
	}
	level3.status.Store(types.INITIALIZING)

	go level3.consume()
	return level3
}

func newLevel3Mailbox(signal *Signal) (*MeasurementMailbox, error) {
	identities := 0

	for _, measured := range signal.Level3 {
		identities += len(measured.IngestRoles())
	}

	capacity := viper.GetInt("market.universe.trading_tier_size") * identities
	return NewMeasurementMailbox(capacity)
}

func (level3 *Level3) Status() types.Status {
	return level3.status.Load().(types.Status)
}

/*
On publishes one immutable raw L3 frame to the shared observed-order ingress.
*/
func (level3 *Level3) On(data []byte) {
	level3.enqueue(channelLevel3, data)
}

/*
OnTrade publishes one immutable raw trade frame to the same owner as L3 so the
shared toxicity engine is never called concurrently across streams.
*/
func (level3 *Level3) OnTrade(data []byte) {
	level3.enqueue(channelTrade, data)
}

func (level3 *Level3) enqueue(stream string, data []byte) {
	if len(data) == 0 || level3.ctx.Err() != nil {
		return
	}

	frame := level3Frame{
		sequence: level3.observed.Add(1),
		stream:   stream,
		raw:      append([]byte(nil), data...),
	}

	for !level3.ring.Push(frame) {
		if level3.ctx.Err() != nil {
			return
		}

		runtime.Gosched()
	}

	select {
	case level3.wake <- struct{}{}:
	default:
	}
}

/*
Measure takes the current immutable measurement for every published identity.
*/
func (level3 *Level3) Measure() ([]*types.Measurement, error) {
	return level3.mailbox.Drain(), nil
}

/*
Close stops ingress and the sole owner loop.
*/
func (level3 *Level3) Close() error {
	level3.cancel()

	for !level3.stopped.Load() {
		runtime.Gosched()
	}

	return level3.ring.Close()
}

func (level3 *Level3) fail(err error) {
	level3.status.Store(types.ERROR)
	errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err))
	level3.cancel()
}
