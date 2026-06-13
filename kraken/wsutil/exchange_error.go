package wsutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ExchangePolicyAction string

const (
	RetrySoon                ExchangePolicyAction = "retry_soon"
	RetryAfter               ExchangePolicyAction = "retry_after"
	RejectOrder              ExchangePolicyAction = "reject_order"
	DisconnectAndResubscribe ExchangePolicyAction = "disconnect_and_resubscribe"
	HaltTrading              ExchangePolicyAction = "halt_trading"
)

type ExchangeError struct {
	Raw        string
	Category   string
	Code       string
	Message    string
	RetryAfter *time.Time
	Malformed  bool
}

func (exchangeError ExchangeError) Error() string {
	if exchangeError.Raw != "" {
		return "kraken websocket: " + exchangeError.Raw
	}

	return "kraken websocket: exchange error"
}

type ExchangePolicyDecision struct {
	Action ExchangePolicyAction
	Delay  time.Duration
}

type ExchangeErrorPolicy struct {
	RetrySoonDelay time.Duration
}

func DefaultExchangeErrorPolicy() ExchangeErrorPolicy {
	return ExchangeErrorPolicy{
		RetrySoonDelay: NewBackoffFromConfig().Initial,
	}
}

func ParseExchangeError(errorText string) ExchangeError {
	raw := strings.TrimSpace(errorText)
	exchangeError := ExchangeError{Raw: raw, Message: raw}

	if raw == "" {
		exchangeError.Category = "unknown"
		exchangeError.Malformed = true
		return exchangeError
	}

	category, detail, hasDetail := strings.Cut(raw, ":")
	exchangeError.Category = strings.TrimSpace(category)

	if !hasDetail {
		exchangeError.Malformed = true
		return exchangeError
	}

	exchangeError.Code, exchangeError.Message = parseExchangeErrorDetail(detail)
	exchangeError.parseRetryAfter(detail)

	return exchangeError
}

func parseExchangeErrorDetail(detail string) (string, string) {
	detail = strings.TrimSpace(detail)

	if detail == "" {
		return "", ""
	}

	code, message, hasMessage := strings.Cut(detail, ":")

	if !hasMessage {
		return detail, detail
	}

	return strings.TrimSpace(code), strings.TrimSpace(message)
}

func (exchangeError *ExchangeError) parseRetryAfter(detail string) {
	if exchangeError.Category != "EService" {
		return
	}

	retryText := strings.TrimSpace(detail)

	if _, message, hasMessage := strings.Cut(retryText, ":"); hasMessage {
		retryText = strings.TrimSpace(message)
	}

	unixTimestamp, parseErr := strconv.ParseInt(retryText, 10, 64)

	if parseErr != nil {
		exchangeError.Malformed = true
		return
	}

	retryAfter := time.Unix(unixTimestamp, 0)

	if !retryAfter.After(time.Now()) {
		exchangeError.Malformed = true
		return
	}

	exchangeError.RetryAfter = &retryAfter
}

func (policy ExchangeErrorPolicy) Classify(
	exchangeError ExchangeError,
) ExchangePolicyDecision {
	if exchangeError.Malformed {
		return ExchangePolicyDecision{Action: HaltTrading}
	}

	switch exchangeError.Category {
	case "EOrder":
		if retryableOrderError(exchangeError) {
			return ExchangePolicyDecision{
				Action: RetrySoon,
				Delay:  policy.retrySoonDelay(),
			}
		}

		return ExchangePolicyDecision{Action: RejectOrder}
	case "EService":
		if exchangeError.RetryAfter != nil {
			return ExchangePolicyDecision{Action: RetryAfter}
		}

		return ExchangePolicyDecision{Action: DisconnectAndResubscribe}
	case "EAPI", "EGeneral":
		return ExchangePolicyDecision{Action: HaltTrading}
	default:
		return ExchangePolicyDecision{Action: HaltTrading}
	}
}

func retryableOrderError(exchangeError ExchangeError) bool {
	text := strings.ToLower(exchangeError.Code + " " + exchangeError.Message)

	return strings.Contains(text, "rate") ||
		strings.Contains(text, "busy") ||
		strings.Contains(text, "temporary")
}

func (policy ExchangeErrorPolicy) retrySoonDelay() time.Duration {
	if policy.RetrySoonDelay > 0 {
		return policy.RetrySoonDelay
	}

	return NewBackoffFromConfig().Initial
}

func HandleExchangeError(ctx context.Context, errorText string) error {
	exchangeError := ParseExchangeError(errorText)
	decision := DefaultExchangeErrorPolicy().Classify(exchangeError)

	return HandleExchangePolicy(ctx, exchangeError, decision)
}

func HandleExchangePolicy(
	ctx context.Context,
	exchangeError ExchangeError,
	decision ExchangePolicyDecision,
) error {
	switch decision.Action {
	case RetrySoon:
		return Wait(ctx, decision.Delay)
	case RetryAfter:
		return waitUntilRetryAfter(ctx, exchangeError)
	case RejectOrder, DisconnectAndResubscribe, HaltTrading:
		return exchangeError
	default:
		return fmt.Errorf("kraken websocket: unhandled exchange policy %q", decision.Action)
	}
}

func waitUntilRetryAfter(
	ctx context.Context,
	exchangeError ExchangeError,
) error {
	if exchangeError.RetryAfter == nil {
		return exchangeError
	}

	return Wait(ctx, time.Until(*exchangeError.RetryAfter))
}
