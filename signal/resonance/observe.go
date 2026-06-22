package resonance

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

func (signal *Signal) hydrateMarketFromTree() {
	if signal == nil || signal.tree == nil {
		return
	}

	signal.ticker.reset()
	signal.book.reset()
	signal.trade.reset()

	for _, role := range []string{"book", "trade", "ticker"} {
		prefix := role + "/"

		for inbound := range signal.tree.Seek([]byte(prefix)) {
			switch role {
			case "book":
				signal.observeBookArtifact(inbound)
			case "trade":
				signal.observeTradeArtifact(inbound)
			case "ticker":
				signal.observeTickerArtifact(inbound)
			}
		}
	}
}

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()
	observedAt := time.Now()

	forEachPayloadElement(artifact.DecryptPayload(), scope, func(symbol string, element []byte) {
		signal.book.ingest(symbol, element, observedAt)
	})
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()
	observedAt := time.Now()

	forEachPayloadElement(artifact.DecryptPayload(), scope, func(symbol string, element []byte) {
		signal.trade.ingest(symbol, element, observedAt)
	})
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	scope, _ := artifact.Scope()
	observedAt := time.Now()

	forEachPayloadElement(artifact.DecryptPayload(), scope, func(symbol string, element []byte) {
		signal.ticker.ingest(symbol, element, observedAt)
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
