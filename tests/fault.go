package tests

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
FaultObservation records the exact deterministic trigger used during a run.
*/
type FaultObservation struct {
	Channel    string                `json:"channel"`
	Occurrence int                   `json:"occurrence"`
	Action     testtypes.FaultAction `json:"action"`
}

/*
FrameObservation preserves generated and delivered bytes for exact replay.
*/
type FrameObservation struct {
	Channel    string        `json:"channel"`
	Occurrence int           `json:"occurrence"`
	Generated  string        `json:"generated"`
	Delivered  []string      `json:"delivered"`
	Delay      time.Duration `json:"delay"`
	Reconnect  bool          `json:"reconnect"`
}

/*
TransportReport separates delivery mechanics from strategy or order economics.
*/
type TransportReport struct {
	Published    int                `json:"published"`
	Dropped      int                `json:"dropped"`
	Duplicated   int                `json:"duplicated"`
	Delayed      int                `json:"delayed"`
	Reordered    int                `json:"reordered"`
	SequenceGaps int                `json:"sequence_gaps"`
	Stale        int                `json:"stale"`
	Malformed    int                `json:"malformed"`
	Reconnects   int                `json:"reconnects"`
	Faults       []FaultObservation `json:"faults"`
	Frames       []FrameObservation `json:"frames"`
}

type faultDelivery struct {
	frames    [][]byte
	delay     time.Duration
	reconnect bool
}

/*
faultInjector applies one seeded fault configuration to a connection.
*/
type faultInjector struct {
	mu          sync.Mutex
	config      testtypes.FaultConfig
	rng         *rand.Rand
	occurrences map[string]int
	previous    map[string][]byte
	held        map[string][]byte
	report      TransportReport
}

func newFaultInjector(config testtypes.FaultConfig) *faultInjector {
	return &faultInjector{
		config:      config,
		rng:         rand.New(rand.NewSource(config.Seed)),
		occurrences: map[string]int{},
		previous:    map[string][]byte{},
		held:        map[string][]byte{},
	}
}

/*
Apply transforms one outbound venue frame according to its exact channel
occurrence. Reordered frames are released immediately after the following
frame, yielding n+1 before n on the real websocket.
*/
func (injector *faultInjector) Apply(
	channel string,
	payload []byte,
) faultDelivery {
	injector.mu.Lock()
	defer injector.mu.Unlock()

	injector.occurrences[channel]++
	occurrence := injector.occurrences[channel]
	delivery := faultDelivery{
		frames: [][]byte{append([]byte{}, payload...)},
		delay:  injector.latency(injector.config.ChannelLatency[channel]),
	}
	rule, found := injector.rule(channel, occurrence)

	if found {
		delivery = injector.applyRule(delivery, rule, channel, occurrence)
	}

	if held, exists := injector.held[channel]; exists &&
		(!found || rule.Action != testtypes.FaultReorder) {
		delivery.frames = append(delivery.frames, held)
		delete(injector.held, channel)
	}

	for _, frame := range delivery.frames {
		injector.previous[channel] = append([]byte{}, frame...)
		injector.report.Published++
	}

	delivered := make([]string, len(delivery.frames))

	for index, frame := range delivery.frames {
		delivered[index] = string(frame)
	}

	injector.report.Frames = append(injector.report.Frames, FrameObservation{
		Channel:    channel,
		Occurrence: occurrence,
		Generated:  string(payload),
		Delivered:  delivered,
		Delay:      delivery.delay,
		Reconnect:  delivery.reconnect,
	})

	return delivery
}

func (injector *faultInjector) rule(
	channel string,
	occurrence int,
) (testtypes.FaultRule, bool) {
	for _, rule := range injector.config.Rules {
		if rule.Channel == channel && rule.Occurrence == occurrence {
			return rule, true
		}
	}

	return testtypes.FaultRule{}, false
}

func (injector *faultInjector) applyRule(
	delivery faultDelivery,
	rule testtypes.FaultRule,
	channel string,
	occurrence int,
) faultDelivery {
	injector.report.Faults = append(injector.report.Faults, FaultObservation{
		Channel: channel, Occurrence: occurrence, Action: rule.Action,
	})

	switch rule.Action {
	case testtypes.FaultDrop:
		delivery.frames = nil
		injector.report.Dropped++
	case testtypes.FaultDuplicate:
		delivery.frames = append(delivery.frames, append([]byte{}, delivery.frames[0]...))
		injector.report.Duplicated++
	case testtypes.FaultDelay:
		delivery.delay += rule.Delay
		injector.report.Delayed++
	case testtypes.FaultReorder:
		injector.held[channel] = delivery.frames[0]
		delivery.frames = nil
		injector.report.Reordered++
	case testtypes.FaultSequenceGap:
		delivery.frames[0] = sequenceGap(delivery.frames[0], rule.SequenceGap)
		injector.report.SequenceGaps++
	case testtypes.FaultStale:
		previous, exists := injector.previous[channel]

		if exists {
			delivery.frames[0] = append([]byte{}, previous...)
		} else {
			delivery.frames = nil
			injector.report.Dropped++
		}

		injector.report.Stale++
	case testtypes.FaultMalformed:
		delivery.frames[0] = []byte("{")

		if len(rule.Payload) > 0 {
			delivery.frames[0] = append([]byte{}, rule.Payload...)
		}

		injector.report.Malformed++
	case testtypes.FaultReconnect:
		delivery.frames = nil
		delivery.reconnect = true
		injector.report.Reconnects++
	}

	return delivery
}

func (injector *faultInjector) latency(config testtypes.LatencyConfig) time.Duration {
	if config.Jitter == 0 {
		return config.Base
	}

	jitter := time.Duration(
		(float64(injector.rng.Int63())/float64(^uint64(0)>>1)*2 - 1) *
			float64(config.Jitter),
	)

	return max(0, config.Base+jitter)
}

func (injector *faultInjector) RESTDelay(path string) time.Duration {
	injector.mu.Lock()
	defer injector.mu.Unlock()

	return injector.latency(injector.config.RESTLatency[path])
}

func (injector *faultInjector) Report() TransportReport {
	injector.mu.Lock()
	defer injector.mu.Unlock()

	report := injector.report
	report.Faults = append([]FaultObservation{}, injector.report.Faults...)
	report.Frames = append([]FrameObservation{}, injector.report.Frames...)

	return report
}

func sequenceGap(payload []byte, gap uint64) []byte {
	var wire map[string]any

	if err := json.Unmarshal(payload, &wire); err != nil {
		panic(fmt.Errorf("simulator: sequence gap requires a JSON object: %w", err))
	}

	observed, known := wire["sequence"].(float64)

	if !known || observed < 0 || observed != math.Trunc(observed) ||
		observed > float64(testtypes.MaximumExactJSONInteger) {
		panic("simulator: sequence gap requires an exact non-negative sequence")
	}
	sequence := uint64(observed)

	if gap > testtypes.MaximumExactJSONInteger-sequence {
		panic("simulator: sequence gap exceeds exact JSON integer range")
	}

	wire["sequence"] = sequence + gap
	encoded, err := json.Marshal(wire)

	if err != nil {
		panic(fmt.Errorf("simulator: encode sequence gap: %w", err))
	}

	return encoded
}
