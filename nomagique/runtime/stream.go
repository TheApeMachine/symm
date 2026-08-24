package runtime

import (
	"fmt"
)

/*
StreamKey is a named topic on the workspace stream. The channel constants in
the types package carry these names, so a node and its downstream stages agree
on a topic without importing each other.
*/
type StreamKey string

/*
Register connects an input topic to a processor and routes its output to an
output topic. The processor is a pure single-unit node: it receives one value
and optionally emits one value, with no awareness of channels, shards, or
locks. The workspace preserves per-key FIFO ordering, so a processor that keeps
its per-symbol state in a keyed numeric unit is thread-safe by construction.

The input and output are the same typed transport channels the rest of the
system already uses, so a pure node wired with Register interoperates with any
legacy stage subscribed to the same named topic.
*/
func Register[In, Out any](
	workspace *Workspace,
	inTopic StreamKey,
	outTopic StreamKey,
	keyExtractor func(In) string,
	outKeyExtractor func(Out) string,
	processor UnitProcessor[In, Out],
) {
	if workspace == nil {
		panic("runtime: workspace required")
	}

	if processor == nil {
		panic("runtime: processor required: " + string(inTopic))
	}

	if keyExtractor == nil {
		panic("runtime: key extractor required: " + string(inTopic))
	}

	input := ChannelOf[In](workspace, string(inTopic), keyExtractor)

	var output *Channel[Out]
	outKey := outKeyExtractor

	if outKey == nil {
		outKey = func(Out) string { return "" }
	}

	if outTopic != "" {
		output = ChannelOf[Out](workspace, string(outTopic), outKey)
	}

	id := fmt.Sprintf("register:%s>%s:%T", inTopic, outTopic, processor)

	input.Subscribe(id, func(value In) error {
		result, emit, err := processor.Process(value)

		if err != nil {
			return err
		}

		if emit && output != nil {
			output.Publish(result)
			workspace.fireTaps(outTopic, outKey(result), result)
		}

		return nil
	})
}

/*
RegisterTap attaches a side-effect observer (audit logging, UI broadcasting,
telemetry) to a topic. Taps run inline with the emitting node but never stall
the hot path; they are fire-and-forget and must not block. A tap observes
every value emitted onto its topic.
*/
func (workspace *Workspace) RegisterTap(topic StreamKey, tap func(key string, data any)) {
	if workspace == nil || tap == nil {
		return
	}

	current, _ := workspace.taps.LoadOrStore(topic, []func(string, any){})
	updated := append(current.([]func(string, any)), tap)
	workspace.taps.Store(topic, updated)
}

/*
Publish injects untyped data for a topic and fires its taps. Typed producers
should keep using ChannelOf(...).Publish(...) for the routing path; this method
is the untyped entry for side-effect observation and one-off emissions.
*/
func (workspace *Workspace) Publish(topic StreamKey, key string, data any) {
	if workspace == nil || workspace.ctx.Err() != nil {
		return
	}

	workspace.fireTaps(topic, key, data)
}

func (workspace *Workspace) fireTaps(topic StreamKey, key string, data any) {
	if workspace == nil {
		return
	}

	if taps, found := workspace.taps.Load(topic); found {
		for _, tap := range taps.([]func(string, any)) {
			tap(key, data)
		}
	}
}
