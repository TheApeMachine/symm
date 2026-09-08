package recording

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/bytedance/sonic"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
	"gocloud.dev/blob"
	"golang.org/x/sync/errgroup"
)

// Session records the original Hindsight records. The bounded queue holds only
// their encoded bytes, so transport and workspace objects can be reused safely.
// A failed write or full queue is terminal and is reported through Errors.
type Session struct {
	Errors        chan error
 Durable chan struct{}
	bucket        *blob.Bucket
	run           hindsight.Run
	sequencer     *hindsight.Sequencer
	mutex         sync.Mutex
	queue         chan pending
	done          chan struct{}
	err           error
	closed        bool
	next          uint64
	phases        map[opportunityWitnessKey]types.OpportunityPhase
	lastWitnessed map[string]time.Time
}

type pending struct {
	key  string
	data []byte
}
type opportunityWitnessKey struct {
	symbol    string
	archetype types.OpportunityArchetype
}

func NewSession(ctx context.Context, bucket *blob.Bucket, run hindsight.Run, capacity, batchSize int, flushInterval time.Duration) (*Session, error) {
	if capacity <= 0 || batchSize <= 0 || batchSize > capacity || flushInterval <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "recording: require positive capacity, batch size within capacity, and flush interval", nil))
	}
	sequencer, err := hindsight.NewSequencer(run.ID)
	if err != nil {
		return nil, err
	}
	if err := store.Write(ctx, bucket, run.ID.Prefix("runs")+"run.json", run); err != nil {
		return nil, err
	}
	session := &Session{Durable: make(chan struct{}, 1), bucket: bucket, run: run, sequencer: sequencer, queue: make(chan pending, capacity), done: make(chan struct{}), Errors: make(chan error, 1), phases: make(map[opportunityWitnessKey]types.OpportunityPhase), lastWitnessed: make(map[string]time.Time)}
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
	frame := hindsight.RawFrame{Identity: identity, Kind: kind, Endpoint: endpoint, ReceivedAt: receivedAt, Payload: payload, PayloadHash: hex.EncodeToString(digest[:])}
	return identity, session.enqueue(identity.Key(), frame)
}

func (session *Session) WriteManifest(manifest hindsight.EnvelopeManifest) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.enqueue(manifest.Envelope.Key("manifests"), manifest)
}

func (session *Session) WriteLearning(event any) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.next++
	return session.enqueue(fmt.Sprintf("%s%020d.json", session.run.ID.Prefix("learning"), session.next), event)
}

func (session *Session) WriteLifecycle(event hindsight.LifecycleEvent) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.next++
	return session.enqueue(fmt.Sprintf("%s%020d.json", session.run.ID.Prefix("lifecycle"), session.next), event)
}

// enqueue is called with mutex held. Encoding freezes the original record;
// persistence never retains the caller's mutable maps, slices, or envelope.
func (session *Session) enqueue(key string, value any) error {
	if session.err != nil {
		return session.err
	}
	if session.closed {
		return errnie.Error(errnie.Err(errnie.IO, "recording: session closed", nil))
	}
	data, err := sonic.Marshal(value)
	if err != nil {
		return session.fail(errnie.Error(errnie.Err(errnie.Validation, "recording: encode "+key, err)))
	}
	select {
	case session.queue <- pending{key: key, data: data}:
		return nil
	default:
		return session.fail(errnie.Error(errnie.Err(errnie.IO, "recording: persistence queue full", nil)))
	}
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
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	batch := make([]pending, 0, batchSize)
	var sequence uint64
	for {
		select {
		case record, open := <-session.queue:
			if open {
				batch = append(batch, record)
			}
			if !open || len(batch) == batchSize {
				sequence++
				if err := session.flush(ctx, batch, sequence); err != nil {
					return
				}
				batch = batch[:0]
			}
			if !open {
				return
			}
		case <-ticker.C:
			sequence++
			if err := session.flush(ctx, batch, sequence); err != nil {
				return
			}
			batch = batch[:0]
		}
	}
}

func (session *Session) flush(ctx context.Context, batch []pending, sequence uint64) error {
	groups := make(map[string][][]byte)
	prefixes := []string{}
	for _, record := range batch {
		parts := strings.SplitN(record.key, "/", 3)
		prefix := parts[0] + "/" + parts[1] + "/"
		if _, found := groups[prefix]; !found {
			prefixes = append(prefixes, prefix)
		}
		groups[prefix] = append(groups[prefix], record.data)
	}
	// A batch's independent record families upload concurrently. The next
	// batch waits for all of them, preserving per-family order with bounded work.
	var uploads errgroup.Group
	for _, prefix := range prefixes {
		uploads.Go(func() error {
			key := fmt.Sprintf("%s%020d.jsonl", prefix, sequence)
			if err := session.bucket.WriteAll(ctx, key, bytes.Join(groups[prefix], []byte("\n")), nil); err != nil {
				return errnie.Error(errnie.Err(errnie.IO, "recording: write batch "+key, err))
			}
			return nil
		})
	}
	if err := uploads.Wait(); err != nil {
		session.mutex.Lock()
		defer session.mutex.Unlock()
		return session.fail(err)
	}
	if len(groups[session.run.ID.Prefix("captures")]) > 0 {
  select { case session.Durable <- struct{}{}: default: } // Coalesced wakeup; captures themselves remain durable.
 }
	clear(batch)
	return nil
}

// Close stops admission and waits until accepted records have been persisted,
// or returns the write failure. The connection remains owned by the application.
func (session *Session) Close() error {
	session.mutex.Lock()
	if !session.closed {
		session.closed = true
		close(session.queue)
	}
	session.mutex.Unlock()
	<-session.done
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.err
}
