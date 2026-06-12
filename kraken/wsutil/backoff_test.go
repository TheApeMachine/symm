package wsutil

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestBackoffWaitHonorsCanceledContext(test *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	backoff := Backoff{
		Initial:    time.Hour,
		Max:        time.Hour,
		Multiplier: 2,
	}

	waitErr := backoff.Wait(ctx, 0)

	if !errors.Is(waitErr, context.Canceled) {
		test.Fatalf("expected context canceled, got %v", waitErr)
	}
}

func TestHandleExchangeErrorRejectsMalformedServiceError(test *testing.T) {
	handleErr := HandleExchangeError(context.Background(), "EService")

	if handleErr == nil {
		test.Fatal("expected malformed service error")
	}
}

func TestHandleExchangeErrorReturnsUnknownError(test *testing.T) {
	handleErr := HandleExchangeError(context.Background(), "EUnknown:bad")

	if handleErr == nil {
		test.Fatal("expected unknown exchange error")
	}
}

func TestParseExchangeErrorServiceRetryAfter(test *testing.T) {
	retryAt := time.Now().UTC().Add(time.Minute).Unix()
	exchangeErr := ParseExchangeError("EService:" + strconv.FormatInt(retryAt, 10))
	decision := DefaultExchangeErrorPolicy().Classify(exchangeErr)

	if exchangeErr.RetryAfter == nil {
		test.Fatal("expected retry-after timestamp")
	}

	if decision.Action != RetryAfter {
		test.Fatalf("expected retry-after decision, got %q", decision.Action)
	}
}

func TestExchangeErrorPolicyRejectsOrderErrors(test *testing.T) {
	exchangeErr := ParseExchangeError("EOrder:Cannot open position")
	decision := DefaultExchangeErrorPolicy().Classify(exchangeErr)

	if decision.Action != RejectOrder {
		test.Fatalf("expected reject-order decision, got %q", decision.Action)
	}
}

func TestExchangeErrorPolicyRetriesRateLimitedOrders(test *testing.T) {
	exchangeErr := ParseExchangeError("EOrder:Rate limit exceeded")
	policy := ExchangeErrorPolicy{RetrySoonDelay: time.Nanosecond}
	decision := policy.Classify(exchangeErr)

	if decision.Action != RetrySoon {
		test.Fatalf("expected retry-soon decision, got %q", decision.Action)
	}

	if decision.Delay != time.Nanosecond {
		test.Fatalf("expected configured retry delay, got %s", decision.Delay)
	}
}

func TestHandleExchangePolicyHonorsCanceledRetry(test *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handleErr := HandleExchangePolicy(
		ctx,
		ParseExchangeError("EOrder:Rate limit exceeded"),
		ExchangePolicyDecision{
			Action: RetrySoon,
			Delay:  time.Hour,
		},
	)

	if !errors.Is(handleErr, context.Canceled) {
		test.Fatalf("expected context canceled, got %v", handleErr)
	}
}

func BenchmarkParseAndClassifyExchangeError(benchmark *testing.B) {
	policy := ExchangeErrorPolicy{RetrySoonDelay: time.Microsecond}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		exchangeErr := ParseExchangeError("EOrder:Rate limit exceeded")
		decision := policy.Classify(exchangeErr)

		if decision.Action != RetrySoon {
			benchmark.Fatal(decision.Action)
		}
	}
}
