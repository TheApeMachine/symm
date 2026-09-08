package recording

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
Session queues Hindsight records for asynchronous persistence into the Iceberg
tables. Producers retain no mutable state in the queue, and write failures
reach Errors.

The lock-free queue stands between producers and the writer deliberately.
Capture runs on the ingest hot path, so it must not contend with whatever the
persist goroutine is doing; the queue converts every producer into a single
uncontended enqueue.
*/
type Session struct {
	Errors    chan error
	Durable   chan struct{}
	writer    *tables.Writer
	run       hindsight.Run
	sequencer *hindsight.Sequencer
	mutex     sync.Mutex
	queue     *lf.Queue[pending]
	wake      chan struct{}
	done      chan struct{}
	err       error
	closed    bool
	next      uint64
	// commitRows and commitInterval govern how often buffered rows become an
	// Iceberg snapshot, which is far coarser than how often the queue drains.
	commitRows     int
	commitInterval time.Duration
	committedAt    time.Time
	uncommitted    bool
	phases         map[opportunityWitnessKey]types.OpportunityPhase
	lastWitnessed  map[string]time.Time
}

/*
pending is one record awaiting persistence, tagged with the table it belongs
to. The row is already converted into its tables row type, so the persist
goroutine only dispatches; it never touches a producer's mutable structures.
*/
type pending struct {
	family string
	row    any
}
type opportunityWitnessKey struct {
	symbol    string
	archetype types.OpportunityArchetype
}

func NewSession(
	ctx context.Context, writer *tables.Writer, run hindsight.Run,
	batchSize int, flushInterval time.Duration,
) (*Session, error) {
	if batchSize <= 0 || flushInterval <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "recording: require positive batch size and flush interval", nil))
	}
	sequencer, err := hindsight.NewSequencer(run.ID)

	if err != nil {
		return nil, err
	}

	writer.AddRun(tables.RunRow{
		ID:             string(run.ID),
		StartedAt:      run.StartedAt,
		CodeCommit:     run.CodeCommit,
		BuildID:        run.BuildID,
		ConfigDigest:   run.ConfigDigest,
		Integrity:      run.Integrity.String(),
		Positions:      int32(run.Positions),
		SchemaVersions: run.SchemaVersions,
	})

	// The run row is committed before anything else is accepted, so a reader
	// never finds captures belonging to a run it cannot describe.
	if err := writer.Commit(ctx); err != nil {
		return nil, err
	}
	session := &Session{
		Durable: make(chan struct{}, 1), Errors: make(chan error, 1),
		writer: writer, run: run, sequencer: sequencer,
		commitRows:     viper.GetInt("hindsight.capture.commit_rows"),
		commitInterval: viper.GetDuration("hindsight.capture.commit_interval"),
		committedAt:    time.Now(),
		queue:          lf.NewQueue[pending](), wake: make(chan struct{}, 1),
		done:          make(chan struct{}),
		phases:        make(map[opportunityWitnessKey]types.OpportunityPhase),
		lastWitnessed: make(map[string]time.Time),
	}
	go session.persist(context.WithoutCancel(ctx), batchSize, flushInterval)
	return session, nil
}

func (session *Session) Capture(kind, endpoint string, payload []byte, receivedAt time.Time, ref hindsight.StreamRef) (hindsight.CaptureIdentity, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if ref.Stream == "" || ref.Epoch == 0 || ref.Sequence == 0 {
		return hindsight.CaptureIdentity{}, errnie.Error(errnie.Err(errnie.Validation, "recording: capture requires transport identity", nil))
	}
	identity, err := session.sequencer.Assign(ref.Stream)

	if err != nil {
		return identity, err
	}
	identity.StreamEpoch, identity.StreamSequence = ref.Epoch, ref.Sequence
	digest := sha256.Sum256(payload)
	return identity, session.enqueue(tables.Captures, tables.CaptureRow{
		Run:            string(identity.Run),
		Sequence:       int64(identity.Sequence),
		Stream:         string(identity.Stream),
		StreamEpoch:    int64(identity.StreamEpoch),
		StreamSequence: int64(identity.StreamSequence),
		ReceivedAt:     receivedAt,
		Endpoint:       endpoint,
		Kind:           kind,
		PayloadHash:    hex.EncodeToString(digest[:]),
		Payload:        payload,
	})
}

func (session *Session) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	return session.enqueue(tables.Manifests, tables.ManifestRow{
		Run:           string(manifest.Envelope.Origin.Run),
		Envelope:      envelopeRow(manifest.Envelope),
		Workload:      manifest.Workload,
		DomainKind:    manifest.DomainKind,
		Symbol:        manifest.Symbol,
		VenueAt:       manifest.VenueAt,
		VenueSequence: manifest.VenueSequence,
	})
}

// envelopeRow converts an envelope reference into its table row form.
func envelopeRow(ref hindsight.EnvelopeRef) tables.EnvelopeRefRow {
	return tables.EnvelopeRefRow{
		Run:      string(ref.Origin.Run),
		Sequence: int64(ref.Origin.Sequence),
		Ordinal:  int64(ref.Ordinal),
	}
}

/*
WriteDecision records one decision as the agent made it, before the tape has
graded it. WriteOutcome records the same decision once a confirmed leg settles
it. The caller converts its own evaluation into a row, because the packages
that produce them sit above this one and cannot be imported from here.
*/
func (session *Session) WriteDecision(row tables.OutcomeRow) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if row.Run == "" {
		row.Run = string(session.run.ID)
	}

	return session.enqueue(tables.Decisions, row)
}

// WriteOutcome records one decision the tape has graded.
func (session *Session) WriteOutcome(row tables.OutcomeRow) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	if row.Run == "" {
		row.Run = string(session.run.ID)
	}

	return session.enqueue(tables.Outcomes, row)
}

func (session *Session) WriteLifecycle(event hindsight.LifecycleEvent) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()

	row := tables.LifecycleRow{
		Run:                 string(session.run.ID),
		DecisionID:          event.DecisionID,
		ActionCorrelationID: event.ActionCorrelationID,
		Symbol:              event.Symbol,
		Kind:                event.Kind,
		Action:              event.Action,
		At:                  event.At,
		CaptureSeq:          int64(event.CaptureSeq),
	}

	if event.Execution != nil {
		row.Exec = executionRow(event.Execution)
	}

	return session.enqueue(tables.Lifecycle, row)
}

/*
enqueue is called with mutex held. The caller has already converted its record
into a row of value types, so nothing mutable crosses into the queue and the
persist goroutine never observes a producer's later edits.
*/
func (session *Session) enqueue(family string, row any) error {
	if session.err != nil {
		return session.err
	}

	if session.closed {
		return errnie.Error(errnie.Err(errnie.IO, "recording: session closed", nil))
	}

	session.queue.Enqueue(pending{family: family, row: row})
	select {
	case session.wake <- struct{}{}:
	default:
	} // Wakeups coalesce; records remain in the queue.
	return nil
}

// fail is called with mutex held and preserves the first failure.
func (session *Session) fail(err error) error {
	if session.err == nil {
		session.err = err
		session.Errors <- err
	}
	return session.err
}

// persist batches encoded originals by record family and run. Each S3 object
// is JSONL; no record is converted into a second storage representation.
func (session *Session) persist(ctx context.Context, batchSize int, interval time.Duration) {
	defer close(session.done)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	batch := make([]pending, 0, batchSize)
	var sequence uint64
	for {
		for len(batch) < batchSize {
			record, open := session.queue.Dequeue()

			if !open {
				break
			}
			batch = append(batch, record)
		}
		session.mutex.Lock()
		drained := session.closed && session.queue.Length() == 0
		session.mutex.Unlock()

		if len(batch) == batchSize || drained {
			sequence++

			if err := session.flush(ctx, batch, sequence); err != nil {
				return
			}
			batch = batch[:0]
			timer.Reset(interval)

			if drained {
				// Nothing further will be accepted, so everything buffered has
				// to reach the table regardless of what it has earned.
				if err := session.commit(ctx, true); err != nil {
					return
				}

				return
			}
			continue
		}

		select {
		case <-session.wake:
		case <-timer.C:
			sequence++

			if err := session.flush(ctx, batch, sequence); err != nil {
				return
			}
			batch = batch[:0]
			timer.Reset(interval)
		}
	}
}

/*
flush moves one drained batch into the writer, then commits if the buffer has
earned a snapshot.

Draining is cheap and happens at ingest cadence; committing is not. An Iceberg
commit writes a snapshot, a manifest, a manifest list and a new metadata file,
so a commit per drain costs more in metadata than the records are worth and
fragments the table. Rows therefore accumulate until they are worth a file.
*/
func (session *Session) flush(ctx context.Context, batch []pending, sequence uint64) error {
	for _, record := range batch {
		switch row := record.row.(type) {
		case tables.CaptureRow:
			session.writer.AddCapture(row)
			session.uncommitted = true
		case tables.ManifestRow:
			session.writer.AddManifest(row)
		case tables.WitnessRow:
			session.writer.AddWitness(row)
		case tables.LifecycleRow:
			session.writer.AddLifecycle(row)
		case tables.OutcomeRow:
			// Decisions and outcomes share a row type, so the family the
			// producer tagged decides which table it lands in.
			if record.family == tables.Decisions {
				session.writer.AddDecision(row)

				continue
			}

			session.writer.AddOutcome(row)
		case tables.GapRow:
			session.writer.AddGap(row)
		default:
			session.mutex.Lock()
			defer session.mutex.Unlock()

			return session.fail(errnie.Error(errnie.Err(
				errnie.Validation, "recording: unknown record family "+record.family, nil,
			)))
		}
	}

	clear(batch)

	return session.commit(ctx, false)
}

/*
commit writes the buffered rows when they are worth a snapshot, or when the
session is closing and everything accepted must reach the table.

Durable is signalled only after captures actually commit. A reader of the
durable tape must not be woken for rows that are still only in memory.
*/
func (session *Session) commit(ctx context.Context, force bool) error {
	pending := session.writer.Pending()

	if pending == 0 {
		return nil
	}

	if !force && pending < session.commitRows &&
		time.Since(session.committedAt) < session.commitInterval {
		return nil
	}

	captured := session.uncommitted

	if err := session.writer.Commit(ctx); err != nil {
		session.mutex.Lock()
		defer session.mutex.Unlock()

		return session.fail(err)
	}

	session.committedAt = time.Now()
	session.uncommitted = false

	if captured {
		select {
		case session.Durable <- struct{}{}:
		default:
		} // Coalesced wakeup; captures themselves remain durable.
	}

	return nil
}

// Close stops admission and waits until accepted records have been persisted,
// or returns the write failure. The connection remains owned by the application.
func (session *Session) Close() error {
	session.mutex.Lock()

	if !session.closed {
		session.closed = true
		select {
		case session.wake <- struct{}{}:
		default:
		}
	}
	session.mutex.Unlock()
	<-session.done
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.err
}

/*
executionRow flattens the venue's execution economics into table columns.

The per-asset fee breakdown is kept as JSON rather than exploded into its own
table: it is a short, rarely-queried tail, and a second table would need a join
for every question about a fill.
*/
func executionRow(execution *kraken.ExecutionData) *tables.ExecutionRow {
	fees := ""

	if len(execution.Fees) > 0 {
		if encoded, err := sonic.Marshal(execution.Fees); err == nil {
			fees = string(encoded)
		}
	}

	return &tables.ExecutionRow{
		OrderID:       execution.OrderID,
		ClientOrderID: execution.ClientOrderID,
		ExecID:        execution.ExecID,
		ExecType:      execution.ExecType,
		TradeID:       int64(execution.TradeID),
		Side:          execution.Side,
		OrderType:     execution.OrderType,
		OrderStatus:   execution.OrderStatus,
		LiquidityInd:  execution.LiquidityInd,
		At:            execution.Timestamp,
		LastQty:       execution.LastQty,
		LastPrice:     execution.LastPrice,
		Cost:          execution.Cost,
		CumQty:        execution.CumQty,
		CumCost:       execution.CumCost,
		AvgPrice:      execution.AvgPrice,
		FeeUsdEquiv:   execution.FeeUsdEquiv,
		Fees:          fees,
	}
}
