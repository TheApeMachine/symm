package mockapi

import (
	"sort"
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Control owns configured responses, recorded requests, subscriptions, and fault
injection separately from transport delivery.
*/
type Control struct {
	mu            sync.Mutex
	writes        [][]byte
	posts         [][]byte
	writeErr      error
	callbackErr   error
	responses     map[string][][]byte
	current       map[string]func() []byte
	postResponses map[string][]byte
	subscriptions map[string]map[string]struct{}
}

/*
Report captures a consumer failure at the transport delivery boundary.
*/
func (control *Control) Report(err error) {
	control.mu.Lock()

	if control.callbackErr == nil {
		control.callbackErr = err
	}

	control.mu.Unlock()
}

/*
Err returns the first consumer failure observed by this connection.
*/
func (control *Control) Err() error {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.callbackErr
}

/*
newControl creates one empty fixture-control surface.
*/
func newControl() *Control {
	return &Control{}
}

/*
RespondPost associates one REST path with its exact fixture payload.
*/
func (control *Control) RespondPost(path string, payload []byte) {
	if path == "" || len(payload) == 0 {
		panic(errnie.Err(errnie.Validation, "tests/mockapi: REST response required", nil))
	}

	control.mu.Lock()
	defer control.mu.Unlock()

	if control.postResponses == nil {
		control.postResponses = map[string][]byte{}
	}

	control.postResponses[path] = append([]byte(nil), payload...)
}

/*
Respond associates a subscription channel with a static fixture payload.
*/
func (control *Control) Respond(channel string, payload []byte) {
	if channel == "" || len(payload) == 0 {
		panic(errnie.Err(errnie.Validation, "tests/mockapi: channel response required", nil))
	}

	control.mu.Lock()
	defer control.mu.Unlock()

	if control.responses == nil {
		control.responses = map[string][][]byte{}
	}

	control.responses[channel] = append(
		control.responses[channel],
		append([]byte(nil), payload...),
	)
}

/*
RespondCurrent supplies a fresh snapshot for each new subscription.
*/
func (control *Control) RespondCurrent(channel string, snapshot func() []byte) {
	if channel == "" || snapshot == nil {
		panic(errnie.Err(errnie.Validation, "tests/mockapi: current response required", nil))
	}

	control.mu.Lock()
	defer control.mu.Unlock()

	if control.current == nil {
		control.current = map[string]func() []byte{}
	}

	control.current[channel] = snapshot
}

/*
Writes returns independent copies of recorded websocket requests.
*/
func (control *Control) Writes() [][]byte {
	control.mu.Lock()
	defer control.mu.Unlock()
	return clonePayloads(control.writes)
}

/*
Posts returns independent copies of recorded REST request bodies.
*/
func (control *Control) Posts() [][]byte {
	control.mu.Lock()
	defer control.mu.Unlock()
	return clonePayloads(control.posts)
}

/*
Subscriptions returns the symbols registered for one channel.
*/
func (control *Control) Subscriptions(channel string) []string {
	control.mu.Lock()
	defer control.mu.Unlock()
	symbols := make([]string, 0, len(control.subscriptions[channel]))

	for symbol := range control.subscriptions[channel] {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}

/*
FailWrites injects an error before any subscription or order side effect.
*/
func (control *Control) FailWrites(err error) {
	control.mu.Lock()
	control.writeErr = err
	control.mu.Unlock()
}

/*
record captures a websocket request before returning its injected failure.
*/
func (control *Control) record(raw []byte) error {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.writes = append(control.writes, append([]byte(nil), raw...))
	return control.writeErr
}

/*
subscribe records symbols and returns the configured static or current source.
*/
func (control *Control) subscribe(
	channel string,
	symbols []string,
) ([][]byte, func() []byte) {
	control.mu.Lock()
	defer control.mu.Unlock()

	if control.subscriptions == nil {
		control.subscriptions = map[string]map[string]struct{}{}
	}

	if control.subscriptions[channel] == nil {
		control.subscriptions[channel] = map[string]struct{}{}
	}

	for _, symbol := range symbols {
		control.subscriptions[channel][symbol] = struct{}{}
	}

	return clonePayloads(control.responses[channel]), control.current[channel]
}

/*
unsubscribe removes only the requested symbols from one venue subscription.
*/
func (control *Control) unsubscribe(channel string, symbols []string) {
	control.mu.Lock()
	defer control.mu.Unlock()

	for _, symbol := range symbols {
		delete(control.subscriptions[channel], symbol)
	}
}

/*
post records one REST request and resolves its explicit fixture route.
*/
func (control *Control) post(path string, raw []byte) ([]byte, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.posts = append(control.posts, append([]byte(nil), raw...))
	payload, configured := control.postResponses[path]

	if !configured {
		return nil, errnie.Err(
			errnie.NotFound,
			"tests/mockapi: REST route not configured: "+path,
			nil,
		)
	}

	return append([]byte(nil), payload...), nil
}

/*
clonePayloads prevents callers from mutating recorded or configured frames.
*/
func clonePayloads(payloads [][]byte) [][]byte {
	clones := make([][]byte, len(payloads))

	for index := range payloads {
		clones[index] = append([]byte(nil), payloads[index]...)
	}

	return clones
}
