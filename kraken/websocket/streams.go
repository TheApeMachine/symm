package websocket

import (
	"sync"

	"github.com/theapemachine/symm/hindsight"
)

/*
streamSpan is one operational connection span within a transport stream: its
epoch and the frame sequence within that epoch. It is owned by the transport
session (not Hindsight) so reconnect invalidation is an operational transport
fact available even when capture is disabled.
*/
type streamSpan struct {
	epoch    hindsight.StreamEpoch
	sequence uint64
}

/*
Streams owns one session's per-stream epoch/sequence bookkeeping, independent of
Hindsight. The transport mints the StreamRef for every frame and bumps epochs on
reconnect; Hindsight records the same fact but is never the source of it.

endpoint names the socket these streams arrive on, so one transport fact has one
stable identity across both the spot and futures sessions.
*/
type Streams struct {
	endpoint string

	mu    sync.Mutex
	spans map[hindsight.Stream]streamSpan
}

func NewStreams(endpoint string) *Streams {
	return &Streams{
		endpoint: endpoint,
		spans:    make(map[hindsight.Stream]streamSpan),
	}
}

/*
Next mints the operational StreamRef for one inbound frame on the given channel
or feed. The stream name mirrors Hindsight's endpoint:kind naming; the epoch
starts at 1 and the sequence within the span is monotonic.
*/
func (streams *Streams) Next(kind string) hindsight.StreamRef {
	if streams == nil {
		return hindsight.StreamRef{}
	}

	stream := hindsight.Stream(streams.endpoint + ":" + kind)

	streams.mu.Lock()
	defer streams.mu.Unlock()

	if streams.spans == nil {
		streams.spans = make(map[hindsight.Stream]streamSpan)
	}

	span := streams.spans[stream]

	if span.epoch == 0 {
		span.epoch = 1
	}

	span.sequence++
	streams.spans[stream] = span

	return hindsight.StreamRef{
		Stream:   stream,
		Epoch:    span.epoch,
		Sequence: span.sequence,
	}
}

/*
Reconnected bumps the operational epoch for every stream this session has seen
and resets each stream's per-epoch sequence. Called on a second (or later)
connection in this process lifetime, completely independent of any capture sink.
*/
func (streams *Streams) Reconnected() {
	if streams == nil {
		return
	}

	streams.mu.Lock()
	defer streams.mu.Unlock()

	for stream, span := range streams.spans {
		span.epoch++
		span.sequence = 0
		streams.spans[stream] = span
	}
}
