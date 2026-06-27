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
	now := time.Now().UTC()

	floor := prev
	if floor <= 1 {
		floor = now.Add(-1 * time.Hour).UnixNano()
	}

	latest := floor

	for _, role := range []string{"book", "trade", "ticker"} {
		for _, seekKey := range hydrateSeekPrefixesDaily(role, floor, now) {
			for inbound := range signal.tree.Seek(seekKey) {
				stamp := inbound.Timestamp()

				if stamp > 0 && stamp <= floor {
					continue
				}

				if stamp > now.UnixNano() {
					break
				}

				r, err := inbound.Role()
				if err != nil || r != role {
					break
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

func hydrateSeekPrefixesDaily(role string, prev int64, now time.Time) [][]byte {
	if prev <= 1 {
		return [][]byte{[]byte(role + "/" + now.UTC().Format("2006/01/02"))}
	}

	start := time.Unix(0, prev).UTC().Truncate(24 * time.Hour)
	end := now.UTC().Truncate(24 * time.Hour)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/(24*time.Hour))+1)

	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		prefixes = append(prefixes, []byte(role+"/"+cursor.Format("2006/01/02")+"/"))
	}

	return prefixes
}

func hydrateSeekPrefixes(role string, prev int64, now time.Time) [][]byte {
	if prev <= 1 {
		// ponytail: before the first observed frame, scan the current minute so
		// startup cannot miss frames written just before the first hydrate. Once
		// a cursor exists, hydration uses second prefixes below.
		return [][]byte{[]byte(role + "/" + now.UTC().Format("2006/01/02/15/04"))}
	}

	start := time.Unix(0, prev).UTC().Truncate(time.Second)
	end := now.UTC().Truncate(time.Second)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/time.Second)+1)

	for cursor := start; !cursor.After(end); cursor = cursor.Add(time.Second) {
		prefixes = append(prefixes, []byte(role+"/"+cursor.Format("2006/01/02/15/04/05")+"/"))
	}

	return prefixes
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
