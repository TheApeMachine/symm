package replay

import (
	"bufio"
	"context"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Capture reads one recorded file and fans frames out to channel subscribers.
*/
type Capture struct {
	path string
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	subs map[string][]chan *public.SocketMessage
}

var active Capture

/*
ActiveCapture is the process-wide replay capture.
*/
func ActiveCapture() *Capture {
	return &active
}

/*
Reset clears cached replay state between tune trials.
*/
func (capture *Capture) Reset() {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	capture.path = ""
	capture.done = nil
	capture.once = sync.Once{}
	capture.subs = nil
}

/*
Done closes after the file has been read once when trading.replay.loop is false.
*/
func (capture *Capture) Done() <-chan struct{} {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	if capture.done == nil {
		capture.done = make(chan struct{})
	}

	return capture.done
}

func (capture *Capture) subscribe(channel string) chan *public.SocketMessage {
	outbound := make(chan *public.SocketMessage, 256)

	capture.mu.Lock()

	if capture.subs == nil {
		capture.subs = make(map[string][]chan *public.SocketMessage)
	}

	capture.subs[channel] = append(capture.subs[channel], outbound)
	capture.mu.Unlock()

	return outbound
}

func (capture *Capture) start(ctx context.Context, path string) {
	capture.mu.Lock()

	if capture.subs == nil || capture.path != path {
		capture.path = path
		capture.done = make(chan struct{})
		capture.once = sync.Once{}
		capture.subs = make(map[string][]chan *public.SocketMessage)
	}

	capture.once.Do(func() {
		go capture.play(ctx, path)
	})

	capture.mu.Unlock()
}

func (capture *Capture) play(ctx context.Context, path string) {
	defer capture.closeSubs()
	defer capture.closeDone()

	file, err := os.Open(path)

	if err != nil {
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	pace := viper.GetViper().GetDuration("trading.replay.pace")

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if pace > 0 {
			time.Sleep(pace)
		}

		capture.emit(scanner.Bytes())
	}
}

func (capture *Capture) emit(line []byte) {
	var envelope public.SocketMessage

	if err := sonic.Unmarshal(line, &envelope); err != nil || envelope.Channel == "" {
		return
	}

	rows, err := envelope.SplitDataRows()

	if err != nil {
		return
	}

	capture.mu.Lock()
	targets := capture.subs[envelope.Channel]
	capture.mu.Unlock()

	for _, row := range rows {
		for _, target := range targets {
			select {
			case target <- row:
			default:
			}
		}
	}
}

func (capture *Capture) closeSubs() {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	for channel, targets := range capture.subs {
		for _, target := range targets {
			close(target)
		}

		capture.subs[channel] = nil
	}
}

func (capture *Capture) closeDone() {
	if viper.GetViper().GetBool("trading.replay.loop") {
		return
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if capture.done != nil {
		close(capture.done)
	}
}
