package client

import (
	"context"
	"fmt"
	"time"
)

/*
WithReplay configures the public client to replay captured websocket frames instead of dialing live Kraken.
*/
func WithReplay(frames [][]byte, pace time.Duration) PublicClientOption {
	return func(publicClient *PublicClient) {
		publicClient.replayFrames = frames
		publicClient.replayPace = pace
	}
}

/*
InjectFrame dispatches one captured websocket payload to registered handlers.
*/
func (publicClient *PublicClient) InjectFrame(ctx context.Context, payload []byte) error {
	publicClient.mu.Lock()
	handlers := append([]func(context.Context, []byte) error(nil), publicClient.handlers...)
	publicClient.mu.Unlock()

	for _, handler := range handlers {
		if err := handler(ctx, payload); err != nil {
			return err
		}
	}

	return nil
}

/*
Replay dispatches captured frames sequentially with optional pacing.
*/
func (publicClient *PublicClient) Replay(ctx context.Context, frames [][]byte, pace time.Duration) error {
	for _, payload := range frames {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := publicClient.InjectFrame(ctx, payload); err != nil {
			return fmt.Errorf("replay frame: %w", err)
		}

		if pace <= 0 {
			continue
		}

		timer := time.NewTimer(pace)

		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return nil
}

func (publicClient *PublicClient) runReplayLoop() {
	if len(publicClient.replayFrames) == 0 {
		return
	}

	_ = publicClient.Replay(publicClient.ctx, publicClient.replayFrames, publicClient.replayPace)
}

/*
StartReplay dispatches configured frames after all handlers are registered.
*/
func (publicClient *PublicClient) StartReplay() {
	if len(publicClient.replayFrames) == 0 {
		return
	}

	go publicClient.runReplayLoop()
}

/*
ReplayMode reports whether the client is configured with captured websocket frames.
*/
func (publicClient *PublicClient) ReplayMode() bool {
	return len(publicClient.replayFrames) > 0
}
