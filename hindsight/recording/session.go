package recording

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/types"
	"gocloud.dev/blob"
)

// Session records the original Hindsight records. The bounded queue holds only
// their encoded bytes, so transport and workspace objects can be reused safely.
// A failed write or full queue is terminal and is reported through Errors.
type Session struct {
	Errors        chan error
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

func NewSession(ctx context.Context, bucket *blob.Bucket, run hindsight.Run, capacity int) (*Session, error) {
	if capacity <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "recording: queue capacity must be positive", nil))
	}
	sequencer, err := hindsight.NewSequencer(run.ID)
	if err != nil {
		return nil, err
	}
	if err := store.Write(ctx, bucket, run.ID.Prefix("runs")+"run.json", run); err != nil {
		return nil, err
	}
	session := &Session{bucket: bucket, run: run, sequencer: sequencer, queue: make(chan pending, capacity), done: make(chan struct{}), Errors: make(chan error, 1), phases: make(map[opportunityWitnessKey]types.OpportunityPhase), lastWitnessed: make(map[string]time.Time)}
	go session.persist(context.WithoutCancel(ctx))
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

func (session *Session) WriteLearning(event hindsight.LearningEvent) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	event.Run = session.run.ID
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
	data, err := json.Marshal(value)
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

func (session *Session) persist(ctx context.Context) {
	defer close(session.done)
	for record := range session.queue {
		if err := session.bucket.WriteAll(ctx, record.key, record.data, nil); err != nil {
			session.mutex.Lock()
			session.fail(errnie.Error(errnie.Err(errnie.IO, "recording: write "+record.key, err)))
			session.mutex.Unlock()
			return
		}
	}
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
