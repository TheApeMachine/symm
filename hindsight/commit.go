package hindsight

import (
	"fmt"
	"io"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

/*
Sample is one labelled closed lot. A B C D are tape ticks. Prices and times
come from the desk lot, because the commit button only sends the ticks.
*/
type Sample struct {
	Run                   uint64
	Symbol                string
	Class                 string
	A, B, C, D            uint64
	EntryPrice, ExitPrice float64
	EntryAt, ExitAt       int64
}

type prefixWriter interface {
	io.Writer
	WriteAtPrefix(prefix string, p []byte) (int, error)
}

/*
Commit stores one detection for a closed virtual lot. The key is
detections/<class>/<symbol>/<epoch>/<c tick>. Train reads that prefix.
*/
func Commit(desk *broker.Desk, writer prefixWriter, body []byte) ([]byte, error) {
	if writer == nil {
		return nil, errnie.Err(errnie.Validation, "[hindsight] store unavailable", nil)
	}

	sample, err := sampleFrom(desk, body)
	if err != nil {
		return nil, err
	}

	value, err := detectionValue(sample)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(writer, value); err != nil {
		return nil, err
	}

	payload, err := sonic.Marshal(map[string]any{
		"marker": map[string]string{
			"a": strconv.FormatUint(sample.A, 10),
			"b": strconv.FormatUint(sample.B, 10),
			"c": strconv.FormatUint(sample.C, 10),
			"d": strconv.FormatUint(sample.D, 10),
		},
	})
	if err != nil {
		return nil, errnie.Err(errnie.Internal, "[hindsight] commit response does not encode", err)
	}

	return payload, nil
}

func sampleFrom(desk *broker.Desk, body []byte) (Sample, error) {
	var request struct {
		Run    string `json:"run"`
		Symbol string `json:"symbol"`
		Class  string `json:"class"`
		A      string `json:"a"`
		B      string `json:"b"`
		C      string `json:"c"`
		D      string `json:"d"`
	}

	if err := sonic.Unmarshal(body, &request); err != nil {
		return Sample{}, errnie.Err(errnie.Validation, "[hindsight] commit body does not decode", err)
	}

	if request.Symbol == "" {
		return Sample{}, errnie.Err(errnie.Validation, "[hindsight] commit has no symbol", nil)
	}

	a, err := parseTick("a", request.A)
	if err != nil {
		return Sample{}, err
	}

	b, err := parseTick("b", request.B)
	if err != nil {
		return Sample{}, err
	}

	c, err := parseTick("c", request.C)
	if err != nil {
		return Sample{}, err
	}

	d, err := parseTick("d", request.D)
	if err != nil {
		return Sample{}, err
	}

	if err := validateAnnotation(request.Class, [4]uint64{a, b, c, d}); err != nil {
		return Sample{}, err
	}

	if desk == nil {
		return Sample{}, errnie.Err(errnie.Validation, "[hindsight] desk unavailable", nil)
	}

	fill, err := desk.ClosedFill(request.Symbol, b, c)
	if err != nil {
		return Sample{}, err
	}

	run := fill.Epoch
	if run == 0 && request.Run != "" {
		parsed, err := strconv.ParseUint(request.Run, 10, 64)
		if err != nil {
			return Sample{}, errnie.Err(errnie.Validation, "[hindsight] commit run is not a tick epoch", err)
		}
		run = parsed
	}

	if run == 0 {
		return Sample{}, errnie.Err(errnie.Validation, "[hindsight] commit has no run epoch", nil)
	}

	return Sample{
		Run:        run,
		Symbol:     request.Symbol,
		Class:      request.Class,
		A:          a,
		B:          b,
		C:          c,
		D:          d,
		EntryPrice: fill.EntryPrice,
		ExitPrice:  fill.ExitPrice,
		EntryAt:    fill.EntryAt,
		ExitAt:     fill.ExitAt,
	}, nil
}

func parseTick(name, raw string) (uint64, error) {
	tick, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || tick == 0 {
		return 0, errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[hindsight] commit %s is not a tape tick", name),
			err,
		)
	}

	return tick, nil
}

func detectionValue(sample Sample) (*data.Value, error) {
	if sample.EntryPrice <= 0 || sample.ExitPrice <= 0 {
		return nil, errnie.Err(errnie.UnprocessableContent, "[hindsight] commit has no entry or exit price", nil)
	}

	value := data.NewValue("detections", sample.Class, sample.Symbol)
	value.Epoch = sample.Run
	value.Tick = sample.C
	value.Prefix = fmt.Sprintf(
		"detections/%s/%s/%019d/%019d",
		sample.Class, sample.Symbol, sample.Run, sample.C,
	)

	a := float64(sample.A)
	b := float64(sample.B)
	c := float64(sample.C)
	d := float64(sample.D)
	entryAt := float64(sample.EntryAt)
	exitAt := float64(sample.ExitAt)

	value.Append([]float64{
		a, a, b, b,
		c, c, d, d,
		sample.EntryPrice, sample.ExitPrice, sample.EntryPrice, sample.ExitPrice,
		entryAt, exitAt,
		b, b, c, c,
		sample.EntryPrice, sample.ExitPrice, entryAt, exitAt,
	}, excursionKeys...)

	if value.Err != nil {
		return nil, value.Err
	}

	return value, nil
}
