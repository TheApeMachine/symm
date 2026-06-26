package resonance

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

func (signal *Signal) HydrateMarketFromTree() {
	if signal == nil || signal.tree == nil {
		return
	}

	prev := atomic.LoadInt64(&signal.lastHydrateStamp)
	now := time.Now().UTC().UnixNano()

	floor := prev
	cursorStamp := prev
	if cursorStamp <= 1 {
		floor = 1
		cursorStamp = now
	}

	h1 := strings.Join(strings.Split(
		datura.FormatTimestamp(cursorStamp), "/",
	)[0:5], "/")

	h2 := strings.Join(strings.Split(
		datura.FormatTimestamp(now), "/",
	)[0:5], "/")

	cursors := []string{h2}
	if h1 != h2 {
		cursors = append(cursors, h1)
	}

	latest := floor

	for _, role := range []string{"book", "trade", "ticker"} {
		for _, cursor := range cursors {
			seekKey := role + "/" + cursor

			for inbound := range signal.tree.Seek([]byte(seekKey)) {
				stamp := inbound.Timestamp()

				if stamp <= floor {
					continue
				}

				switch role {
				case "book":
					signal.observeBookArtifact(inbound)
				case "trade":
					signal.observeTradeArtifact(inbound)
				case "ticker":
					signal.observeTickerArtifact(inbound)
				}

				if stamp > latest {
					latest = stamp
				}
			}
		}
	}

	if latest > floor {
		atomic.StoreInt64(&signal.lastHydrateStamp, latest)
	}
}

func treeArtifactPayload(artifact *datura.Artifact) []byte {
	if artifact == nil || !artifact.HasPayload() {
		return nil
	}

	return artifact.DecryptPayload()
}

func artifactObservedAt(artifact *datura.Artifact, element []byte) time.Time {
	observed, ok := elementTime(element, "timestamp")

	if ok && !observed.IsZero() {
		return observed
	}

	if artifact != nil && artifact.Timestamp() > 0 {
		return time.Unix(0, artifact.Timestamp()).UTC()
	}

	return time.Now().UTC()
}

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()

	forEachPayloadElement(treeArtifactPayload(artifact), scope, func(symbol string, element []byte) {
		signal.book.ingest(symbol, element, artifactObservedAt(artifact, element))
		signal.markMarketChanged(symbol)
	})
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()

	forEachPayloadElement(treeArtifactPayload(artifact), scope, func(symbol string, element []byte) {
		signal.trade.ingest(symbol, element, artifactObservedAt(artifact, element))
		signal.markMarketChanged(symbol)
	})
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()

	forEachPayloadElement(treeArtifactPayload(artifact), scope, func(symbol string, element []byte) {
		signal.ticker.ingest(symbol, element, artifactObservedAt(artifact, element))
		signal.markMarketChanged(symbol)
	})
}

func forEachPayloadElement(
	payload []byte,
	fallbackScope string,
	visit func(symbol string, element []byte),
) {
	if len(payload) == 0 || visit == nil {
		return
	}

	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}

	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Data) > 0 {
		for _, row := range envelope.Data {
			if len(row) == 0 {
				continue
			}

			element := append([]byte(nil), row...)
			visit(elementSymbol(element, fallbackScope), element)
		}

		return
	}

	var rows []json.RawMessage

	if json.Unmarshal(payload, &rows) == nil && len(rows) > 0 {
		for _, row := range rows {
			if len(row) == 0 {
				continue
			}

			element := append([]byte(nil), row...)
			visit(elementSymbol(element, fallbackScope), element)
		}

		return
	}

	element := append([]byte(nil), payload...)
	visit(elementSymbol(element, fallbackScope), element)
}

func elementSymbol(element []byte, fallbackScope string) string {
	symbol, ok := peekElementOK[string](element, "symbol")

	if ok && symbol != "" {
		return symbol
	}

	return fallbackScope
}

func peekElementOK[T any](element []byte, path string) (T, bool) {
	var zero T

	if len(element) == 0 {
		return zero, false
	}

	segments := strings.Split(path, ".")
	pathArgs := make([]any, len(segments))

	for index, segment := range segments {
		if arrayIndex, err := strconv.Atoi(segment); err == nil {
			pathArgs[index] = arrayIndex

			continue
		}

		pathArgs[index] = segment
	}

	node, err := sonic.Get(element, pathArgs...)

	if err != nil || !node.Exists() {
		return zero, false
	}

	artifact := datura.Acquire("element", datura.Artifact_Type_json)

	if artifact.WithPayload(element) == nil {
		artifact.Release()

		return zero, false
	}

	peekPath := make([]any, len(segments))

	for index, segment := range segments {
		if arrayIndex, err := strconv.Atoi(segment); err == nil {
			peekPath[index] = arrayIndex

			continue
		}

		peekPath[index] = segment
	}

	value := datura.Peek[T](artifact, peekPath...)
	artifact.Release()

	return value, true
}

func elementTime(element []byte, key string) (time.Time, bool) {
	return peekElementOK[time.Time](element, key)
}
