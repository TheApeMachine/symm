package hindsight

import (
	"context"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
Measurements streams *data.Value from the store.
*/
func Measurements(client *store.Client, prefixes ...string) iter.Seq[*data.Value] {
	return store.Values(client, prefixes...)
}

type Run struct {
	Epoch     int64     `json:"epoch"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
}

type Service struct {
	ctx          context.Context
	client       *store.Client
	fragmentsMu  sync.RWMutex
	fragments    []TrainedFragmentResponse
	fragmentsAt  time.Time
	fragmentsMap map[int64]*data.Value
}

func NewService(ctx context.Context, client *store.Client) *Service {
	return &Service{
		ctx:          ctx,
		client:       client,
		fragmentsMap: make(map[int64]*data.Value),
	}
}

func (service *Service) Register(router fiber.Router) {
	if service == nil || router == nil {
		return
	}

	router.Get("/hindsight/runs", service.runs)

	router.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	router.Get("/hindsight/timeline", websocket.New(service.timeline, websocket.Config{
		Origins: []string{"*"},
	}))

	router.Get("/hindsight/symbols", service.symbols)
	router.Get("/hindsight/excursions", service.excursions)
	router.Get("/hindsight/data", service.data)
	router.Get("/hindsight/captures", service.captures)
	router.Get("/hindsight/envelope", service.envelope)

	router.Get("/training/fragments", service.trainingFragments)
	router.Get("/training/fragments/:id/points", service.trainingFragmentPoints)

	router.Get("/hindsight/gaps", service.gaps)
	router.Get("/hindsight/lifecycle", service.lifecycle)
	router.Get("/hindsight/states", service.states)
	router.Get("/hindsight/state", service.state)
	router.Get("/hindsight/metric-map", service.metricMap)
}

func (service *Service) metricMap(ctx fiber.Ctx) error {
	return ctx.JSON(fiber.Map{
		"metrics": []any{},
		"signals": fiber.Map{},
	})
}

func (service *Service) runs(reqCtx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	runs := []Run{}
	seen := make(map[int64]bool)

	for page := range service.client.List("run/epoch/", "/") {
		for _, commonPrefix := range page.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			prefixStr := strings.TrimPrefix(*commonPrefix.Prefix, "run/epoch/")
			prefixStr = strings.Trim(prefixStr, "/")
			epoch, err := strconv.ParseInt(prefixStr, 10, 64)

			if err != nil || epoch <= 0 {
				continue
			}

			if seen[epoch] {
				continue
			}

			seen[epoch] = true
			runs = append(runs, Run{
				Epoch:     epoch,
				StartedAt: time.Unix(0, epoch).UTC(),
				Status:    "recorded",
			})
		}
	}

	return reqCtx.JSON(runs)
}

func (service *Service) timeline(conn *websocket.Conn) {
	if service.client == nil {
		return
	}

	run := conn.Query("run")

	if run == "" {
		run = conn.Query("epoch")
	}

	if run == "" {
		return
	}

	epoch := uint64(parseInt64Query(run))
	symbol := conn.Query("symbol")
	fromTick := uint64(parseInt64Query(conn.Query("from")))
	toTick := uint64(parseInt64Query(conn.Query("to")))

	const timelineBatchSize = 256
	batch := make([]*data.Value, 0, timelineBatchSize)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		if fromTick > 0 && val.Tick < fromTick {
			continue
		}

		if toTick > 0 && val.Tick > toTick {
			continue
		}

		batch = append(batch, val)

		if len(batch) < timelineBatchSize {
			continue
		}

		payload, err := sonic.Marshal(batch)

		if err != nil {
			errnie.Error(err)
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			errnie.Error(err)
			return
		}

		batch = batch[:0]
	}

	if len(batch) > 0 {
		payload, err := sonic.Marshal(batch)

		if err != nil {
			errnie.Error(err)
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			errnie.Error(err)
			return
		}
	}
}

func (service *Service) symbols(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	symbols := []string{}
	seen := make(map[string]bool)

	prefix := "spot:trade/"

	if epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/*/%019d/", epoch)
	}

	for page := range service.client.List(prefix, "/") {
		for _, commonPrefix := range page.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			parts := strings.Split(strings.Trim(*commonPrefix.Prefix, "/"), "/")

			if len(parts) < 2 {
				continue
			}

			symbol := parts[1]

			if !seen[symbol] {
				seen[symbol] = true
				symbols = append(symbols, symbol)
			}
		}
	}

	return ctx.JSON(symbols)
}

func (service *Service) excursions(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	excursions := []*data.Value{}

	for val := range store.Values(service.client, "detector/") {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		status := val.Role
		if len(val.Layers) > 0 {
			if s := data.Attribute[string](val.Layers[0], "status"); s != "" {
				status = s
			}
		}

		if status == "resolved" {
			excursions = append(excursions, val)
		}
	}

	return ctx.JSON(excursions)
}

func (service *Service) data(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	epoch := uint64(parseInt64Query(ctx.Query("epoch")))
	limit := int(parseUintQuery(ctx.Query("limit")))
	var values []*data.Value

	for val := range store.Values(service.client, "spot:trade/") {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		values = append(values, val)

		if limit > 0 && len(values) >= limit {
			break
		}
	}

	if values == nil {
		values = []*data.Value{}
	}

	return ctx.JSON(values)
}

func (service *Service) captures(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	after := uint64(parseInt64Query(ctx.Query("after")))
	symbol := ctx.Query("symbol")

	const maxCaptures = 64
	captures := make([]map[string]any, 0, maxCaptures)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		if val.SeqIdx < after {
			continue
		}

		receivedAt := ""
		if val.Timestamp > 0 {
			receivedAt = time.Unix(0, int64(val.Timestamp)).UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		captures = append(captures, map[string]any{
			"identity": map[string]any{
				"run":            strconv.FormatUint(val.Epoch, 10),
				"sequence":       val.SeqIdx,
				"stream":         val.Origin,
				"streamEpoch":    val.Epoch,
				"streamSequence": val.SeqIdx,
			},
			"kind":       "ticker",
			"endpoint":   val.Origin,
			"receivedAt": receivedAt,
		})

		if len(captures) >= maxCaptures {
			break
		}
	}

	return ctx.JSON(captures)
}

func (service *Service) envelope(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	seq := uint64(parseInt64Query(ctx.Query("seq")))
	symbol := ctx.Query("symbol")

	if seq <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "seq required")
	}

	var found *data.Value

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		if val.SeqIdx == seq && (symbol == "" || val.Scope == symbol) {
			found = val
			break
		}
	}

	if found == nil {
		return fiber.NewError(fiber.StatusNotFound, "frame not found")
	}

	payloadBytes, err := sonic.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(HindsightEnvelope{
		Payload: string(payloadBytes),
	})
}

type HindsightEnvelope struct {
	Payload string `json:"payload"`
}

type HindsightGap struct {
	RunId    string `json:"runId"`
	Encoding string `json:"encoding"`
	Sequence int64  `json:"sequence"`
	Detail   string `json:"detail"`
}

type HindsightLifecycleEvent struct {
	DecisionId string `json:"decisionId"`
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Action     string `json:"action"`
	At         string `json:"at"`
	CaptureSeq uint64 `json:"captureSeq"`
}

type HindsightCaptureIdentity struct {
	Run            string `json:"run"`
	Sequence       uint64 `json:"sequence"`
	Stream         string `json:"stream"`
	StreamEpoch    uint64 `json:"streamEpoch"`
	StreamSequence uint64 `json:"streamSequence"`
}

type HindsightStateEnvelope struct {
	Origin  HindsightCaptureIdentity `json:"origin"`
	Ordinal int                      `json:"ordinal"`
}

type HindsightState struct {
	Envelope HindsightStateEnvelope `json:"envelope"`
	Payload  string                 `json:"payload"`
}

func (service *Service) gaps(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightGap{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	symbol := ctx.Query("symbol")
	gaps := make([]HindsightGap, 0)
	var prevSeq int64 = -1

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		if prevSeq != -1 && int64(val.SeqIdx) > prevSeq+1 {
			gaps = append(gaps, HindsightGap{
				RunId:    strconv.FormatUint(val.Epoch, 10),
				Encoding: "sequence_gap",
				Sequence: prevSeq + 1,
				Detail:   fmt.Sprintf("Sequence jump from %d to %d (missing %d frames)", prevSeq, val.SeqIdx, int64(val.SeqIdx)-prevSeq-1),
			})
		}

		prevSeq = int64(val.SeqIdx)
	}

	return ctx.JSON(gaps)
}

func (service *Service) lifecycle(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightLifecycleEvent{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	symbol := ctx.Query("symbol")
	events := make([]HindsightLifecycleEvent, 0)

	detPrefix := "detector/"

	if symbol != "" && epoch > 0 {
		detPrefix = fmt.Sprintf("detector/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, detPrefix) {
		if val == nil || val.Origin != "detector" {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		action := val.Role
		if len(val.Layers) > 0 {
			if t := data.Attribute[string](val.Layers[0], "type"); t != "" {
				action = t
			}
		}

		atStr := ""
		if val.Timestamp > 0 {
			atStr = time.Unix(0, int64(val.Timestamp)).UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		events = append(events, HindsightLifecycleEvent{
			DecisionId: fmt.Sprintf("det-%d-%d", val.Epoch, val.SeqIdx),
			Symbol:     val.Scope,
			Kind:       "detector",
			Action:     action,
			At:         atStr,
			CaptureSeq: val.SeqIdx,
		})
	}

	polPrefix := "policy/current/"

	if epoch > 0 {
		polPrefix = fmt.Sprintf("policy/current/%019d/", epoch)
	}

	keys := service.client.ListKeys(polPrefix)
	if len(keys) == 0 && symbol != "" && epoch > 0 {
		polPrefix = fmt.Sprintf("policy/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, polPrefix) {
		if val == nil || val.Origin != "policy" {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		action := val.Role

		atStr := ""
		if val.Timestamp > 0 {
			atStr = time.Unix(0, int64(val.Timestamp)).UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		events = append(events, HindsightLifecycleEvent{
			DecisionId: fmt.Sprintf("pol-%d-%d", val.Epoch, val.SeqIdx),
			Symbol:     val.Scope,
			Kind:       "policy",
			Action:     action,
			At:         atStr,
			CaptureSeq: val.SeqIdx,
		})
	}

	return ctx.JSON(events)
}

func (service *Service) states(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightState{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	symbol := ctx.Query("symbol")
	const maxStates = 64
	states := make([]HindsightState, 0, maxStates)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		payloadBytes, err := sonic.Marshal(val)

		if err != nil {
			errnie.Error(errnie.Err(errnie.UnprocessableContent, "[hindsight] state marshal failed for "+val.Prefix, err))
			continue
		}

		states = append(states, HindsightState{
			Envelope: HindsightStateEnvelope{
				Origin: HindsightCaptureIdentity{
					Run:            strconv.FormatUint(val.Epoch, 10),
					Sequence:       val.SeqIdx,
					Stream:         val.Origin,
					StreamEpoch:    val.Epoch,
					StreamSequence: val.SeqIdx,
				},
				Ordinal: 0,
			},
			Payload: string(payloadBytes),
		})

		if len(states) >= maxStates {
			break
		}
	}

	return ctx.JSON(states)
}

func (service *Service) state(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := uint64(parseInt64Query(run))
	seq := uint64(parseInt64Query(ctx.Query("seq")))
	ordinal := int(parseUintQuery(ctx.Query("ordinal")))
	symbol := ctx.Query("symbol")

	if seq <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "seq required")
	}

	var found *data.Value
	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for val := range store.Values(service.client, prefix) {
		if val == nil {
			continue
		}

		if epoch > 0 && val.Epoch != epoch {
			continue
		}

		if val.SeqIdx == seq && (symbol == "" || val.Scope == symbol) {
			found = val
			break
		}
	}

	if found == nil {
		return fiber.NewError(fiber.StatusNotFound, "no witnessed state at sequence")
	}

	payloadBytes, err := sonic.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(HindsightState{
		Envelope: HindsightStateEnvelope{
			Origin: HindsightCaptureIdentity{
				Run:            strconv.FormatUint(found.Epoch, 10),
				Sequence:       found.SeqIdx,
				Stream:         found.Origin,
				StreamEpoch:    found.Epoch,
				StreamSequence: found.SeqIdx,
			},
			Ordinal: ordinal,
		},
		Payload: string(payloadBytes),
	})
}

func readFailure(err error) error {
	return fiber.NewError(fiber.StatusInternalServerError, errnie.Error(err).Error())
}

func parseUintQuery(raw string) uint64 {
	value, err := strconv.ParseUint(raw, 10, 64)

	if err != nil {
		return 0
	}

	return value
}

func parseInt64Query(raw string) int64 {
	value, err := strconv.ParseInt(raw, 10, 64)

	if err != nil {
		return 0
	}

	return value
}

type FragmentPoint struct {
	X    int     `json:"x"`
	Y    float64 `json:"y"`
	Seq  int64   `json:"seq"`
	Time int64   `json:"time"`
}

type TrainedFragmentResponse struct {
	Id                int64           `json:"id"`
	Symbol            string          `json:"symbol"`
	Epoch             int64           `json:"epoch"`
	MarkA             int64           `json:"mark_a"`
	MarkB             int64           `json:"mark_b"`
	MarkC             int64           `json:"mark_c"`
	EntryPrice        float64         `json:"entry_price"`
	ExitPrice         float64         `json:"exit_price"`
	Magnitude         float64         `json:"magnitude"`
	Direction         string          `json:"direction"`
	Class             string          `json:"class,omitempty"`
	Tokens            []string        `json:"tokens"`
	Points            []FragmentPoint `json:"points"`
	EntryIdx          int64           `json:"entry_idx"`
	ExitIdx           int64           `json:"exit_idx"`
	PredictedEntryIdx int64           `json:"predicted_entry_idx"`
	PredictedExitIdx  int64           `json:"predicted_exit_idx"`
	LearnedAt         string          `json:"learned_at"`
}

func parseTick(val *data.Value, key string) int64 {
	if val == nil {
		return 0
	}
	if len(val.Layers) > 0 {
		if t := data.Attribute[int64](val.Layers[0], key); t != 0 {
			return t
		}
	}
	return int64(val.Tick)
}

func parsePrice(val *data.Value, key string) float64 {
	if val == nil {
		return 0
	}
	if len(val.Layers) > 0 {
		if p := data.Attribute[float64](val.Layers[0], key); p != 0 {
			return p
		}
	}
	return 0
}

func filterFragmentsByEpoch(fragments []TrainedFragmentResponse, epoch int64) []TrainedFragmentResponse {
	if epoch <= 0 {
		return fragments
	}

	filtered := make([]TrainedFragmentResponse, 0)
	for _, f := range fragments {
		if f.Epoch == epoch {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

func (service *Service) getFragments(epoch int64) ([]TrainedFragmentResponse, error) {
	if service.client == nil {
		return []TrainedFragmentResponse{}, nil
	}

	service.fragmentsMu.RLock()
	if len(service.fragments) > 0 && time.Since(service.fragmentsAt) < 30*time.Second {
		cached := service.fragments
		service.fragmentsMu.RUnlock()
		return filterFragmentsByEpoch(cached, epoch), nil
	}
	service.fragmentsMu.RUnlock()

	service.fragmentsMu.Lock()
	defer service.fragmentsMu.Unlock()

	if len(service.fragments) > 0 && time.Since(service.fragmentsAt) < 30*time.Second {
		return filterFragmentsByEpoch(service.fragments, epoch), nil
	}

	fragments := make([]TrainedFragmentResponse, 0)
	fragmentsMap := make(map[int64]*data.Value)
	var autoID int64 = 1

	for det := range store.Values(service.client, "detector/") {
		if det == nil || det.Origin != "detector" {
			continue
		}

		id := autoID
		autoID++

		fragmentsMap[id] = det

		startTick := parseTick(det, "start_tick")
		bTick := parseTick(det, "b_tick")
		cTick := parseTick(det, "c_tick")
		entryPrice := parsePrice(det, "b_price")
		exitPrice := parsePrice(det, "c_price")

		var magnitude float64

		if entryPrice > 0 {
			magnitude = (exitPrice - entryPrice) / entryPrice
		}

		direction := "up"

		if exitPrice < entryPrice {
			direction = "down"
		}

		class := det.Role
		if len(det.Layers) > 0 {
			if t := data.Attribute[string](det.Layers[0], "type"); t != "" {
				class = t
			}
		}

		learnedAt := ""
		if det.Timestamp > 0 {
			learnedAt = time.Unix(0, int64(det.Timestamp)).UTC().Format("2006-01-02T15:04:05Z07:00")
		}

		var startIdx int64
		var bIdx int64
		var cIdx int64

		if len(det.Layers) > 0 {
			startIdx = data.Attribute[int64](det.Layers[0], "start_idx")
			bIdx = data.Attribute[int64](det.Layers[0], "b_idx")
			cIdx = data.Attribute[int64](det.Layers[0], "c_idx")
		}

		entryIdx := max(bIdx-startIdx, 0)
		exitIdx := max(cIdx-startIdx, entryIdx)

		var predictedEntryIdx int64 = -1
		var predictedExitIdx int64 = -1

		if len(det.Layers) > 0 {
			if pe := data.Attribute[int64](det.Layers[0], "predicted_entry"); pe >= 0 {
				predictedEntryIdx = pe
			}
			if px := data.Attribute[int64](det.Layers[0], "predicted_exit"); px >= 0 {
				predictedExitIdx = px
			}
		}

		fragments = append(fragments, TrainedFragmentResponse{
			Id:                id,
			Symbol:            det.Scope,
			Epoch:             int64(det.Epoch),
			MarkA:             startTick,
			MarkB:             bTick,
			MarkC:             cTick,
			EntryPrice:        entryPrice,
			ExitPrice:         exitPrice,
			Magnitude:         magnitude,
			Direction:         direction,
			Class:             class,
			Tokens:            []string{},
			Points:            []FragmentPoint{},
			EntryIdx:          entryIdx,
			ExitIdx:           exitIdx,
			PredictedEntryIdx: predictedEntryIdx,
			PredictedExitIdx:  predictedExitIdx,
			LearnedAt:         learnedAt,
		})
	}

	service.fragments = fragments
	service.fragmentsMap = fragmentsMap
	service.fragmentsAt = time.Now()

	return filterFragmentsByEpoch(fragments, epoch), nil
}

func (service *Service) trainingFragments(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]TrainedFragmentResponse{})
	}

	epoch := parseInt64Query(ctx.Query("epoch"))
	fragments, err := service.getFragments(epoch)

	if err != nil {
		return err
	}

	return ctx.JSON(fragments)
}

func fetchFragmentPoints(client *store.Client, symbol string, epoch, startTick, cTick int64) []FragmentPoint {
	if client == nil {
		return []FragmentPoint{}
	}

	points := make([]FragmentPoint, 0)
	var x int
	pointsPrefix := fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)

	for m := range store.Values(client, pointsPrefix) {
		if m == nil {
			continue
		}

		if startTick > 0 && int64(m.Tick) < startTick {
			continue
		}

		if cTick > 0 && int64(m.Tick) > cTick {
			break
		}

		var price float64
		if len(m.Layers) > 0 {
			price = data.Attribute[float64](m.Layers[0], "price")
			if price == 0 {
				price = data.Attribute[float64](m.Layers[0], "data", 0, "price")
			}
		}

		if price <= 0 {
			continue
		}

		var timeMs int64
		if m.Timestamp > 0 {
			timeMs = int64(m.Timestamp / 1e6)
			if m.Timestamp < 1e11 {
				timeMs = int64(m.Timestamp * 1000)
			}
		}

		points = append(points, FragmentPoint{
			X:    x,
			Y:    price,
			Seq:  int64(m.Tick),
			Time: timeMs,
		})

		x++
	}

	return points
}

func (service *Service) trainingFragmentPoints(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]FragmentPoint{})
	}

	targetID := parseInt64Query(ctx.Params("id"))

	service.fragmentsMu.RLock()
	targetDet := service.fragmentsMap[targetID]
	service.fragmentsMu.RUnlock()

	if targetDet == nil {
		if _, err := service.getFragments(0); err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "[hindsight] fragments unavailable", err))
		}
		service.fragmentsMu.RLock()
		targetDet = service.fragmentsMap[targetID]
		service.fragmentsMu.RUnlock()
	}

	if targetDet == nil {
		return ctx.JSON([]FragmentPoint{})
	}

	startTick := parseTick(targetDet, "start_tick")
	cTick := parseTick(targetDet, "c_tick")

	if startTick <= 0 && targetDet.Tick > 0 {
		startTick = int64(targetDet.Tick)
	}

	if cTick <= 0 && targetDet.SeqIdx > 0 {
		cTick = int64(targetDet.SeqIdx)
	}

	points := fetchFragmentPoints(service.client, targetDet.Scope, int64(targetDet.Epoch), startTick, cTick)
	return ctx.JSON(points)
}
