package replay

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

const maxChannelBuffer = 16384

/*
Hub replays one JSONL capture in file order.
*/
type Hub struct {
	path     string
	rest     map[string]json.RawMessage
	meta     map[string]json.RawMessage
	done     chan struct{}
	doneOnce sync.Once
	start    sync.Once
	mu       sync.Mutex
	channels map[string]*channelState
}

type channelState struct {
	buffer [][]byte
	subs   []chan []byte
}

var sharedHub struct {
	mu   sync.Mutex
	path string
	hub  *Hub
}

/*
Open returns the shared Hub for path.
*/
func Open(path string) (*Hub, error) {
	sharedHub.mu.Lock()
	defer sharedHub.mu.Unlock()

	if sharedHub.hub != nil && sharedHub.path == path {
		return sharedHub.hub, nil
	}

	hub := &Hub{
		path:     path,
		rest:     make(map[string]json.RawMessage),
		meta:     make(map[string]json.RawMessage),
		done:     make(chan struct{}),
		channels: make(map[string]*channelState),
	}

	if err := hub.indexStatic(); err != nil {
		return nil, err
	}

	sharedHub.path = path
	sharedHub.hub = hub

	return hub, nil
}

/*
Done closes after the first full playback when ReplayLoop is false.
*/
func (hub *Hub) Done() <-chan struct{} {
	return hub.done
}

/*
RESTBody returns a recorded REST payload for channel, if present.
*/
func (hub *Hub) RESTBody(channel string) (json.RawMessage, bool) {
	body, ok := hub.rest[channel]

	return body, ok
}

/*
Meta returns recorded metadata for channel, if present.
*/
func (hub *Hub) Meta(channel string) (json.RawMessage, bool) {
	body, ok := hub.meta[channel]

	return body, ok
}

/*
SubscribeWS registers a consumer for inbound frames on channel.
*/
func (hub *Hub) SubscribeWS(channel string) <-chan []byte {
	outbound := make(chan []byte, 256)

	hub.mu.Lock()
	state := hub.channels[channel]

	if state == nil {
		state = &channelState{}
		hub.channels[channel] = state
	}

	state.subs = append(state.subs, outbound)

	for _, frame := range state.buffer {
		select {
		case outbound <- frame:
		default:
		}
	}

	hub.mu.Unlock()

	hub.start.Do(func() {
		go hub.runPlayback()
	})

	return outbound
}

func (hub *Hub) indexStatic() error {
	file, err := os.Open(hub.path)

	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var line Line

		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		switch line.Transport {
		case TransportREST:
			hub.rest[line.Channel] = line.Payload
		case TransportMeta:
			hub.meta[line.Channel] = line.Payload
		}
	}

	return scanner.Err()
}

func (hub *Hub) runPlayback() {
	for {
		hub.playOnce()

		if !config.System.ReplayLoop {
			hub.doneOnce.Do(func() { close(hub.done) })

			return
		}

		hub.resetBuffers()
	}
}

func (hub *Hub) resetBuffers() {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	for _, state := range hub.channels {
		state.buffer = nil
	}
}

func (hub *Hub) playOnce() {
	file, err := os.Open(hub.path)

	if err != nil {
		hub.closeAllStreams()

		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	pace := config.System.ReplayPace
	var previous time.Time

	for scanner.Scan() {
		var line Line

		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		if line.Transport != TransportWS {
			continue
		}

		if line.Direction != "" && line.Direction != DirectionIn {
			continue
		}

		if pace > 0 && !previous.IsZero() && !line.Timestamp.IsZero() {
			delay := line.Timestamp.Sub(previous)

			if delay > 0 && delay < 30*time.Second {
				time.Sleep(delay)
			} else {
				time.Sleep(pace)
			}
		} else if pace > 0 {
			time.Sleep(pace)
		}

		previous = line.Timestamp
		hub.dispatch(line.Channel, append([]byte(nil), line.Payload...))
	}

	hub.closeAllStreams()
}

func (hub *Hub) dispatch(channel string, payload []byte) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	state := hub.channels[channel]

	if state == nil {
		state = &channelState{}
		hub.channels[channel] = state
	}

	if len(state.subs) == 0 {
		if len(state.buffer) < maxChannelBuffer {
			state.buffer = append(state.buffer, payload)
		}

		return
	}

	for _, target := range state.subs {
		target <- payload
	}
}

func (hub *Hub) closeAllStreams() {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	for _, state := range hub.channels {
		for _, target := range state.subs {
			close(target)
		}

		state.subs = nil
	}
}
