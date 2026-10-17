package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/theapemachine/symm/data"
)

func TestSortKeysChronological(t *testing.T) {
	padded := []string{
		objectKey(10, 1),
		objectKey(2, 9),
		objectKey(2, 1),
		objectKey(2, 1),
	}
	got := append([]string(nil), padded...)
	SortKeys(got)

	want := []string{objectKey(2, 1), objectKey(2, 1), objectKey(2, 9), objectKey(10, 1)}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("padded order\n%s", strings.Join(got, "\n"))
	}

	sameTick := []string{"b/role/scope/1/9.bin", "a/role/scope/1/9.bin"}
	SortKeys(sameTick)

	if sameTick[0] != "a/role/scope/1/9.bin" {
		t.Fatalf("tie break = %v", sameTick)
	}

	unusual := []string{"o/r/s/9/1.bin", "o/r/s/10/1.bin", "noslash", "only/tail"}
	SortKeys(unusual)

	if unusual[0] != "noslash" || unusual[1] != "o/r/s/10/1.bin" || unusual[2] != "o/r/s/9/1.bin" || unusual[3] != "only/tail" {
		t.Fatalf("string order of unusual keys = %v", unusual)
	}
}

func TestReadBody(t *testing.T) {
	sized := int64(4)
	got, err := readBody(io.NopCloser(bytes.NewReader([]byte("abcd"))), &sized)

	if err != nil || string(got) != "abcd" || cap(got) != len(got) {
		t.Fatalf("sized read %q cap %d err %v", got, cap(got), err)
	}

	got, err = readBody(io.NopCloser(bytes.NewReader([]byte("xyz"))), nil)

	if err != nil || string(got) != "xyz" {
		t.Fatalf("unknown length %q err %v", got, err)
	}

	_, err = readBody(io.NopCloser(bytes.NewReader([]byte("ab"))), &sized)

	if err == nil {
		t.Fatal("short body returned success")
	}
}

func TestListAndValues(t *testing.T) {
	mem := newMemS3("bucket")
	bodies := map[string][]byte{}

	for _, tick := range []uint64{3, 1, 2} {
		for _, epoch := range []uint64{2, 1} {
			value := sampleValue(epoch, tick, []byte("tick"))
			mem.put(value.Prefix, value.Bytes())
			bodies[value.Prefix] = value.Bytes()
		}
	}

	odd := []byte("odd")
	mem.put("odd/key", odd)
	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 4)

	keys := client.ListKeys(data.TickPrefix(), data.TickPrefix(1))
	seen := map[string]int{}

	for _, key := range keys {
		seen[key]++
	}

	if len(keys) != 6 {
		t.Fatalf("deduped keys = %d %#v", len(keys), keys)
	}

	for key, n := range seen {
		if n != 1 {
			t.Fatalf("duplicate %s", key)
		}
	}

	var previous string

	for _, key := range keys {
		if previous != "" && strings.Compare(rankOf(previous), rankOf(key)) > 0 {
			t.Fatalf("%s before %s", key, previous)
		}

		previous = key
	}

	var got [][2]uint64

	for value, err := range Values(client, data.TickPrefix()) {
		if err != nil {
			t.Fatal(err)
		}

		got = append(got, [2]uint64{value.Epoch, value.Tick})

		if string(value.Layers[0]) != "tick" {
			t.Fatalf("layer %q", value.Layers[0])
		}
	}

	want := [][2]uint64{{1, 1}, {1, 2}, {1, 3}, {2, 1}, {2, 2}, {2, 3}}

	if len(got) != len(want) {
		t.Fatalf("order %#v", got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %#v", got)
		}
	}

	epochs, err := Epochs(client, data.TickPrefix())

	if err != nil {
		t.Fatal(err)
	}

	if len(epochs) != 2 || epochs[0] != 1 || epochs[1] != 2 {
		t.Fatalf("epochs %#v", epochs)
	}
}

func TestListKeysPaginationProgressAndEmpty(t *testing.T) {
	mem := newMemS3("bucket")

	for i := range 1200 {
		mem.put(objectKey(1, uint64(i)), sampleValue(1, uint64(i)+1, []byte{byte(i)}).Bytes())
	}

	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 8)
	var pages []int
	keys := client.ListKeysProgress(func(n int) { pages = append(pages, n) }, data.TickPrefix(1), data.TickPrefix(1))

	if len(keys) != 1200 {
		t.Fatalf("len = %d, lists = %d", len(keys), mem.lists.Load())
	}

	if mem.lists.Load() < 2 || len(pages) < 2 {
		t.Fatalf("lists %d pages %#v", mem.lists.Load(), pages)
	}

	if keys[0] != objectKey(1, 0) || keys[1199] != objectKey(1, 1199) {
		t.Fatalf("endpoints %s %s", keys[0], keys[1199])
	}

	emptyMem := newMemS3("bucket")
	emptyServer := newMockServer(emptyMem)
	defer emptyServer.Close()
	empty := testClient(t, emptyServer, 2)

	if len(empty.ListKeys()) != 0 {
		t.Fatal("empty prefix listed keys")
	}

	n := 0

	for _, err := range Values(empty) {
		if err != nil {
			t.Fatal(err)
		}

		n++
	}

	if n != 0 {
		t.Fatalf("empty values %d", n)
	}

	for _, err := range ParallelValues(client, nil) {
		t.Fatalf("empty keys yielded %v", err)
	}
}

func TestListingAndReadErrors(t *testing.T) {
	mem := newMemS3("bucket")
	mem.put(objectKey(1, 1), sampleValue(1, 1, []byte("ok")).Bytes())
	mem.put(objectKey(1, 2), []byte("not-a-value"))
	mem.failBody[objectKey(1, 3)] = true
	mem.put(objectKey(1, 3), bytes.Repeat([]byte("x"), 32))
	mem.status = func(_ *http.Request, key string) int {
		if key == objectKey(1, 4) {
			return http.StatusInternalServerError
		}

		return 0
	}
	mem.put(objectKey(1, 4), sampleValue(1, 4, []byte("nope")).Bytes())
	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 4)

	_, err := ReadValue(client, objectKey(1, 2))

	if err == nil {
		t.Fatal("garbage decoded")
	}

	_, err = ReadValue(client, objectKey(1, 3))

	if err == nil {
		t.Fatal("truncated body decoded")
	}

	_, err = ReadValue(client, objectKey(1, 4))

	if err == nil {
		t.Fatal("status error decoded")
	}

	mem.listFail.Store(true)
	values := 0
	var listed error

	for _, err := range Values(client, data.TickPrefix(1)) {
		if err != nil {
			listed = err
			break
		}

		values++
	}

	if listed == nil || values != 0 {
		t.Fatalf("values %d err %v", values, listed)
	}

	epochs, err := Epochs(client, data.TickPrefix())

	if err == nil || epochs != nil {
		t.Fatalf("epochs %#v err %v", epochs, err)
	}

	gates, err := PolicyGates(client)

	if err == nil || gates != nil {
		t.Fatalf("gates %#v err %v", gates, err)
	}
}

func TestCancellationDeadlineAndEarlyStop(t *testing.T) {
	mem := newMemS3("bucket")

	for i := range 20 {
		mem.put(objectKey(1, uint64(i)), sampleValue(1, uint64(i)+1, []byte("x")).Bytes())
	}

	mem.delay = func(string) time.Duration { return 5 * time.Second }
	server := newMockServer(mem)
	defer server.Close()
	transport := benchTransport(4, 16<<10, 16<<10)
	ctx, cancel := context.WithCancel(context.Background())
	client := newMockClient(ctx, server, 4, transport)
	t.Cleanup(func() {
		cancel()
		transport.CloseIdleConnections()
		server.Close()
	})

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	var canceled error
	got := 0

	for _, err := range Values(client, data.TickPrefix(1)) {
		if err != nil {
			canceled = err
			break
		}

		got++
	}

	if !errors.Is(canceled, context.Canceled) || got == 20 {
		t.Fatalf("got %d err %v", got, canceled)
	}

	deadlineCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	deadlineClient := newMockClient(deadlineCtx, server, 2, transport)
	var expired error

	for _, err := range Values(deadlineClient, data.TickPrefix(1)) {
		if err != nil {
			expired = err
			break
		}
	}

	if !errors.Is(expired, context.DeadlineExceeded) {
		t.Fatalf("deadline err %v", expired)
	}

	liveCtx, liveCancel := context.WithCancel(context.Background())
	defer liveCancel()
	live := newMockClient(liveCtx, server, 2, transport)
	seen := 0

	for _, err := range Values(live, data.TickPrefix(1)) {
		if err != nil {
			t.Fatal(err)
		}

		seen++

		if seen == 1 {
			break
		}
	}

	transport.CloseIdleConnections()
	waitWorkers(t)
	mem.delay = nil
	done := make(chan struct{})

	go func() {
		defer close(done)

		for _, err := range Values(live, data.TickPrefix(1)) {
			if err != nil && !errors.Is(err, context.Canceled) {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second read blocked after early stop")
	}
}

func TestOpenStream(t *testing.T) {
	mem := newMemS3("bucket")
	var want []byte

	for i := range 3 {
		body := sampleValue(1, uint64(i)+1, bytes.Repeat([]byte{byte('a' + i)}, 50)).Bytes()
		mem.put(objectKey(1, uint64(i)), body)
		want = append(want, body...)
	}

	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 3)
	stream := Open(client, data.TickPrefix(1))
	buf := make([]byte, 7)
	var got []byte

	for {
		n, err := stream.Read(buf)

		got = append(got, buf[:n]...)

		if err == io.EOF {
			break
		}

		if err != nil {
			t.Fatal(err)
		}

		if n == 0 {
			t.Fatal("empty read before EOF")
		}
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("stream %d bytes, want %d", len(got), len(want))
	}

	n, err := stream.Read(buf)

	if n != 0 || err != io.EOF {
		t.Fatalf("after EOF n=%d err=%v", n, err)
	}

	again := Open(client, data.TickPrefix(1))
	all, err := io.ReadAll(again)

	if err != nil || !bytes.Equal(all, want) {
		t.Fatalf("ReadAll %d err %v", len(all), err)
	}

	if err := again.Close(); err != nil {
		t.Fatal(err)
	}

	n, err = Open(client, data.TickPrefix(1)).Read(nil)

	if n != 0 || err != nil {
		t.Fatalf("empty buffer n=%d err=%v", n, err)
	}
}

func TestNilClient(t *testing.T) {
	var got error

	for _, err := range Values(nil) {
		got = err
	}

	if got == nil {
		t.Fatal("nil Values succeeded")
	}

	stream := Open(nil)
	_, err := stream.Read(make([]byte, 4))

	if err == nil {
		t.Fatal("nil Open read succeeded")
	}

	if _, err := stream.Write([]byte{1}); err == nil {
		t.Fatal("nil Open write succeeded")
	}

	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadValue(nil, "k"); err == nil {
		t.Fatal("nil ReadValue succeeded")
	}
}

func TestSharedClientConcurrency(t *testing.T) {
	mem := newMemS3("bucket")

	for i := range 12 {
		mem.put(objectKey(1, uint64(i)), sampleValue(1, uint64(i)+1, []byte("z")).Bytes())
	}

	mem.delay = func(string) time.Duration { return 40 * time.Millisecond }
	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 2)
	mem.maxInflight.Store(0)
	var group sync.WaitGroup
	group.Add(2)
	errCh := make(chan error, 2)

	consume := func(read func() error) {
		defer group.Done()
		errCh <- read()
	}

	consume(func() error {
		n := 0

		for value, err := range Values(client, data.TickPrefix(1)) {
			if err != nil {
				return err
			}

			n++
			_ = value
		}

		if n != 12 {
			return errors.New("short values")
		}

		return nil
	})
	consume(func() error {
		stream := Open(client, data.TickPrefix(1))
		defer stream.Close()
		_, err := io.ReadAll(stream)
		return err
	})
	group.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	if mem.maxInflight.Load() > 2 {
		t.Fatalf("max inflight %d", mem.maxInflight.Load())
	}
}

func TestParallelValuesProgressAndReorder(t *testing.T) {
	mem := newMemS3("bucket")

	for i := range 30 {
		mem.put(objectKey(1, uint64(i)), sampleValue(1, uint64(i)+1, []byte{byte(i)}).Bytes())
	}

	mem.delay = func(key string) time.Duration {
		if strings.HasSuffix(key, "0000000000000000000.bin") || strings.Contains(key, "/0000000000000000005.bin") {
			return 30 * time.Millisecond
		}

		return 0
	}
	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 4)
	var steps atomic.Int32
	var ticks []uint64

	for value, err := range ParallelValuesProgress(client, client.ListKeys(data.TickPrefix(1)), func() {
		steps.Add(1)
	}) {
		if err != nil {
			t.Fatal(err)
		}

		ticks = append(ticks, value.Tick)
	}

	if len(ticks) != 30 || steps.Load() != 30 {
		t.Fatalf("ticks %d steps %d", len(ticks), steps.Load())
	}

	for i := range ticks {
		if ticks[i] != uint64(i+1) {
			t.Fatalf("order %#v", ticks)
		}
	}
}

func TestPolicyGatesDedup(t *testing.T) {
	mem := newMemS3("bucket")
	mem.put(policyObject("enter", 3))
	mem.put(policyObject("exit", 3))
	mem.put(policyObject("noop", 4))
	server := newMockServer(mem)
	defer server.Close()
	client := testClient(t, server, 4)
	gates, err := PolicyGates(client)

	if err != nil {
		t.Fatal(err)
	}

	if len(gates) != 2 || gates[0].Epoch != 3 || gates[1].Epoch != 4 {
		t.Fatalf("gates %#v", gates)
	}
}

func TestCallerHTTPClientWins(t *testing.T) {
	marker := &http.Client{Timeout: 123 * time.Second}
	custom := "http://example.test"
	options := s3.Options{}

	options.HTTPClient = marker
	options.BaseEndpoint = aws.String(custom)
	options.UsePathStyle = false

	if options.HTTPClient != marker {
		t.Fatal("caller HTTP client was replaced")
	}

	if aws.ToString(options.BaseEndpoint) != custom || options.UsePathStyle {
		t.Fatalf("endpoint %s path-style %v", aws.ToString(options.BaseEndpoint), options.UsePathStyle)
	}
}

func TestListCancelIsNotSuccess(t *testing.T) {
	mem := newMemS3("bucket")
	mem.listDelay = 300 * time.Millisecond
	mem.put(objectKey(1, 1), sampleValue(1, 1, []byte("x")).Bytes())
	server := newMockServer(mem)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	client := newMockClient(ctx, server, 2, nil)
	t.Cleanup(cancel)

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	var got error

	for _, err := range Values(client, data.TickPrefix(1)) {
		got = err
	}

	if !errors.Is(got, context.Canceled) {
		t.Fatalf("err %v gets %d", got, mem.gets.Load())
	}
}

func testClient(t *testing.T, server *httptest.Server, inflight int) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return newMockClient(ctx, server, inflight, benchTransport(inflight, 32<<10, 32<<10))
}

func sampleValue(epoch, tick uint64, payload []byte) *data.Value {
	value := data.NewValue("workspace", "tick", "aggregate")
	value.Epoch = epoch
	value.Tick = tick
	value.FormatPrefix()
	value.Append(append([]byte(nil), payload...))
	return value
}

func policyObject(action string, epoch uint64) (string, []byte) {
	value := data.NewValue(data.PolicyOrigin, action, data.PolicyScope)
	value.Epoch = epoch
	value.Tick = 1
	value.FormatPrefix()

	if err := data.AppendPolicyGate(value, data.PolicyGate{
		TickEpoch:    epoch,
		Enabled:      true,
		SurpriseZ:    1.5,
		LearningRate: 0.1,
		Warmup:       data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: 0.2, SlowRate: 0.05},
	}); err != nil {
		panic(err)
	}

	return value.Prefix, value.Bytes()
}

func rankOf(key string) string {
	rest, tick, _ := cutLast(key)
	_, epoch, _ := cutLast(rest)
	return epoch + "\x00" + tick + "\x00" + key
}

func waitWorkers(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		var stacks bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
		text := stacks.String()

		if !strings.Contains(text, "parallelObjects.func") && !strings.Contains(text, "prefixStream.run") {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("download workers still running")
}
