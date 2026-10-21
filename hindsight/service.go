package hindsight

import (
	"context"
	"fmt"
	"io"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
Service serves the dashboard's hindsight REST: the run list, a tape scan, a
single commit, and the whole-run commit. Commit is the label boundary — a
commit moves draft detections into detections/, and symm train reads nothing
else.
*/
type Service struct {
	ctx      context.Context
	client   *store.Client
	scanMu   sync.Mutex
	tapesMu  sync.Mutex
	tapes    map[uint64]*runTape
	tapeWait map[uint64]chan struct{}
	tapeErr  map[uint64]error
}

func NewService(ctx context.Context, client *store.Client) (*Service, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[hindsight] a store client is required", nil)
	}

	return &Service{
		ctx:      ctx,
		client:   client,
		tapes:    map[uint64]*runTape{},
		tapeWait: map[uint64]chan struct{}{},
		tapeErr:  map[uint64]error{},
	}, nil
}

/*
Register mounts the hindsight routes on router.
*/
func (service *Service) Register(router fiber.Router) {
	router.Post("/hindsight/commit", service.commitRoute)
	router.Post("/hindsight/commit-all", service.commitAllRoute)
	router.Post("/hindsight/scan", service.scanRoute)
	router.Get("/hindsight/runs", func(ctx fiber.Ctx) error {
		runs, err := service.Runs()

		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, errnie.Error(err).Error())
		}

		return ctx.JSON(runs)
	})
}

type Run struct {
	ID        string `json:"id"`
	StartedAt string `json:"started_at"`
}

/*
Runs lists every epoch with stored ticks, newest first.
*/
func (service *Service) Runs() ([]Run, error) {
	type entry struct {
		epoch uint64
		run   Run
	}

	found := make(map[uint64]bool)
	runs := make([]entry, 0)

	for key := range service.client.ListKeys("workspace/tick/aggregate/") {
		epoch, err := excursionEpoch(key)

		if err != nil || found[epoch] {
			continue
		}

		found[epoch] = true
		runs = append(runs, entry{
			epoch: epoch,
			run: Run{
				ID:        strconv.FormatUint(epoch, 10),
				StartedAt: time.Unix(0, int64(epoch)).UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			},
		})
	}

	slices.SortFunc(runs, func(a, b entry) int {
		return cmpEpoch(b.epoch, a.epoch)
	})

	out := make([]Run, 0, len(runs))

	for _, item := range runs {
		out = append(out, item.run)
	}

	return out, nil
}

func cmpEpoch(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}

	return 0
}

/*
prices reads one symbol's last print per tick between two ticks, for the
learned-retrace recalculation after a hand-placed commit.
*/
func prices(tape *runTape, symbol string, from, to uint64) []float64 {
	if tape == nil {
		return nil
	}

	series := make([]float64, 0)

	for _, row := range tape.bySymbol[symbol] {
		if row.frame == nil || row.tick == nil || len(row.tick.Trades) == 0 {
			continue
		}

		if row.frame.Tick < from || row.frame.Tick > to {
			continue
		}

		last := row.tick.Trades[len(row.tick.Trades)-1]

		series = append(series, last.Price.Float64())
	}

	return series
}

/*
tape loads one run, single-flighted: a caller whose context is canceled does
not publish that cancellation onto the waiters.
*/
func (service *Service) tape(ctx context.Context, run uint64) (*runTape, error) {
	for {
		service.tapesMu.Lock()

		if loaded := service.tapes[run]; loaded != nil {
			service.tapesMu.Unlock()
			return loaded, nil
		}

		if wait := service.tapeWait[run]; wait != nil {
			service.tapesMu.Unlock()

			select {
			case <-wait:
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			continue
		}

		wait := make(chan struct{})
		service.tapeWait[run] = wait
		service.tapesMu.Unlock()

		loaded, err := LoadRun(ctx, service.client, run, nil)

		service.tapesMu.Lock()

		if err == nil {
			service.tapes[run] = loaded
			delete(service.tapeErr, run)
		} else {
			service.tapeErr[run] = err
		}

		delete(service.tapeWait, run)
		service.tapesMu.Unlock()

		close(wait)

		if err != nil {
			return nil, err
		}

		return loaded, nil
	}
}

/*
commit is the label boundary: the hand-placed window stores as a detection
under detections/<class>/<symbol>, the learned retrace is saved, the run's
drafts are recalculated from the tape held in memory, and the next widest
draft for the symbol returns so the operator marks the next excursion
without another download.
*/
func (service *Service) commit(ctx context.Context, run uint64, symbol, class string, marks [4]uint64) (int, [4]uint64, bool, error) {
	none := [4]uint64{}

	if !service.scanMu.TryLock() {
		return 0, none, false, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return 0, none, false, err
	}

	priorRetrace := LearnedRetrace(service.client)
	retrace := math.Max(priorRetrace, includedRetrace(prices(loaded, symbol, marks[1], marks[2])))
	retraceChanged := retrace > priorRetrace+1e-12

	if _, err := service.writeMarks("detections", run, symbol, class, marks, markedTrades(loaded, symbol, marks[0], marks[3])); err != nil {
		return 0, none, false, err
	}

	if err := saveLearnedRetrace(service.client, retrace); err != nil {
		return 0, none, false, err
	}

	var drafts []*data.Value

	if retraceChanged {
		drafts, err = service.rescan(ctx, run, retrace)
	} else {
		drafts, err = service.rescanSymbols(ctx, run, retrace, []string{symbol})
	}

	if err != nil {
		return 0, none, false, err
	}

	if err := WriteScanStamp(service.client, run, retrace, len(drafts)); err != nil {
		return 0, none, false, err
	}

	a, b, c, d, ok := widestDraft(drafts, symbol, class)

	return len(drafts), [4]uint64{a, b, c, d}, ok, nil
}

/*
commitAll promotes every stored draft of the run into detections/.
*/
func (service *Service) commitAll(ctx context.Context, run uint64) (int, error) {
	if !service.scanMu.TryLock() {
		return 0, errnie.Err(errnie.NotAcceptable, "[hindsight] a scan is already running", nil)
	}

	defer service.scanMu.Unlock()

	written := 0

	for _, class := range excursionClasses {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		for value, err := range service.values("draft/" + class + "/") {
			if err != nil {
				return written, err
			}

			if value.Epoch != run {
				continue
			}

			/*
				Commit is a copy, not a move: the stored draft keeps its layers
				(whose indices name the draft key); the detection's key is the
				promoted identity — origin/role/scope/epoch.
			*/
			key := detectionKey(class, value.Scope, run)

			if _, err := service.client.WriteAtPrefix(key, value.Bytes()); err != nil {
				return written, err
			}

			written++
		}
	}

	return written, nil
}

/*
values streams stored objects under prefix as Values.
*/
func (service *Service) values(prefix string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		collected := &objects{}

		if err := service.client.Stream(service.ctx, prefix, collected); err != nil {
			yield(nil, err)
			return
		}

		for _, value := range collected.values {
			if !yield(value, nil) {
				return
			}
		}
	}
}

/*
rescan reruns every symbol tape already in memory and replaces the run's
drafts.
*/
func (service *Service) rescan(ctx context.Context, run uint64, retrace float64) ([]*data.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return nil, err
	}

	drafts := ScanRun(loaded, retrace, nil)

	if err := StoreDrafts(service.client, run, drafts); err != nil {
		return nil, err
	}

	return drafts, nil
}

/*
rescanSymbols replays only the given symbols: with an unchanged learned
retrace, only the edited symbol's drafts need refresh.
*/
func (service *Service) rescanSymbols(ctx context.Context, run uint64, retrace float64, symbols []string) ([]*data.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	loaded, err := service.tape(ctx, run)

	if err != nil {
		return nil, err
	}

	want := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		want[symbol] = struct{}{}
	}

	drafts := make([]*data.Value, 0)

	for symbol := range want {
		drafts = append(drafts, scanSymbol(loaded, symbol, retrace)...)
	}

	if err := replaceDrafts(service.client, run, want, drafts); err != nil {
		return nil, err
	}

	return drafts, nil
}

/*
replaceDrafts drops the run's drafts for want (every symbol when want is
empty) and writes drafts in their place.
*/
func replaceDrafts(client *store.Client, run uint64, want map[string]struct{}, drafts []*data.Value) error {
	for _, class := range excursionClasses {
		for key := range client.ListKeys("draft/" + class + "/") {
			got, err := excursionEpoch(key)

			if err != nil || got != run {
				continue
			}

			if len(want) > 0 {
				symbol := strings.Join(strings.Split(key, "/")[3:], "/")

				if _, ok := want[symbol]; !ok {
					continue
				}
			}

			if err := client.DeletePrefix(key); err != nil {
				return err
			}
		}
	}

	for _, value := range drafts {
		if err := writeDraftTo(client, value); err != nil {
			return err
		}
	}

	return nil
}

/*
detectionKey is the store key one excursion of symbol in run: the epoch is
zero-padded so keys list in tape order.
*/
func detectionKey(class, symbol string, run uint64) string {
	return "detections/" + class + "/" + symbol + "/" + fmt.Sprintf("%019d", run)
}

/*
writeMarks stores one hand-placed detection under origin.
*/
func (service *Service) writeMarks(origin string, run uint64, symbol, class string, marks [4]uint64, trades []annotationTrade) (string, error) {
	if service == nil || service.client == nil {
		return "", errnie.Err(errnie.Validation, "[hindsight] commit needs a store", nil)
	}

	vector, err := annotationVector(class, marks, trades)

	if err != nil {
		return "", err
	}

	value := data.NewValue(origin, class, symbol)
	value.Epoch = run
	value.Tick = marks[2]
	value.Append(vector, excursionKeys...)
	value.Prefix = detectionKey(class, symbol, run)

	if _, err := io.Copy(service.client, value); err != nil {
		return "", err
	}

	return value.Prefix, nil
}

/*
saveLearnedRetrace teaches the detector the pullback the last commit kept
inside one swing. The next detect run reads this one object.
*/
func saveLearnedRetrace(client *store.Client, retrace float64) error {
	if client == nil {
		return nil
	}

	value := data.NewValue("draft", "params", "learned")
	value.Append([]float64{retrace}, "retrace")

	_, err := io.Copy(client, value)

	return err
}

func widestDraft(drafts []*data.Value, symbol, class string) (a, b, c, d uint64, ok bool) {
	best := 0.0

	for _, value := range drafts {
		if value == nil || value.Scope != symbol || value.Role != class {
			continue
		}

		fields := metrics(value)

		if fields["b_price"] <= 0 {
			continue
		}

		span := math.Abs(fields["c_price"] - fields["b_price"])

		if ok && span <= best {
			continue
		}

		if fields["a_tick"] < 1 {
			fields["a_tick"] = fields["b_tick"]
		}

		best = span
		a = uint64(fields["a_tick"])
		b = uint64(fields["b_tick"])
		c = uint64(fields["c_tick"])
		d = uint64(fields["d_tick"])
		ok = true
	}

	return a, b, c, d, ok
}

type commitRequest struct {
	Run    string `json:"run"`
	Symbol string `json:"symbol"`
	Class  string `json:"class"`
	A      string `json:"a"`
	B      string `json:"b"`
	C      string `json:"c"`
	D      string `json:"d"`
}

func (service *Service) commitRoute(ctx fiber.Ctx) error {
	var request commitRequest

	if err := sonic.Unmarshal(ctx.Body(), &request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "commit body is not JSON")
	}

	run, err := strconv.ParseUint(request.Run, 10, 64)

	if err != nil || request.Symbol == "" {
		return fiber.NewError(fiber.StatusBadRequest, "commit needs a run and a symbol")
	}

	marks := [4]uint64{}

	for idx, raw := range []string{request.A, request.B, request.C, request.D} {
		marks[idx], err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "commit ticks must be decimal strings")
		}
	}

	written, next, ok, err := service.commit(ctx.Context(), run, request.Symbol, request.Class, marks)

	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	response := struct {
		Drafts int `json:"drafts"`
		Marker *struct {
			A string `json:"a"`
			B string `json:"b"`
			C string `json:"c"`
			D string `json:"d"`
		} `json:"marker,omitempty"`
	}{Drafts: written}

	if ok {
		response.Marker = &struct {
			A string `json:"a"`
			B string `json:"b"`
			C string `json:"c"`
			D string `json:"d"`
		}{
			A: strconv.FormatUint(next[0], 10),
			B: strconv.FormatUint(next[1], 10),
			C: strconv.FormatUint(next[2], 10),
			D: strconv.FormatUint(next[3], 10),
		}
	}

	return ctx.JSON(response)
}

func (service *Service) commitAllRoute(ctx fiber.Ctx) error {
	run, err := runOf(ctx.Body())

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	written, err := service.commitAll(ctx.Context(), run)

	if err != nil {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}

	return ctx.JSON(struct {
		Committed int `json:"committed"`
	}{Committed: written})
}

func (service *Service) scanRoute(ctx fiber.Ctx) error {
	run, err := runOf(ctx.Body())

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if !service.scanMu.TryLock() {
		return fiber.NewError(fiber.StatusConflict, "[hindsight] a scan is already running")
	}

	defer service.scanMu.Unlock()

	retrace := LearnedRetrace(service.client)
	drafts, err := service.rescan(ctx.Context(), run, retrace)

	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	if err := WriteScanStamp(service.client, run, retrace, len(drafts)); err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}

	return ctx.JSON(struct {
		Drafts int `json:"drafts"`
	}{Drafts: len(drafts)})
}

func runOf(body []byte) (uint64, error) {
	var request struct {
		Run string `json:"run"`
	}

	if err := sonic.Unmarshal(body, &request); err != nil {
		return 0, errnie.Err(errnie.Validation, "body is not JSON", err)
	}

	run, err := strconv.ParseUint(request.Run, 10, 64)

	if err != nil || run == 0 {
		return 0, errnie.Err(errnie.Validation, "run is required", err)
	}

	return run, nil
}

var _ = fmt.Sprintf
