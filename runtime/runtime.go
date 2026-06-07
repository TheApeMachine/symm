package runtime

import (
	"context"
	"fmt"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
)

/*
Runtime holds live engine state: market caches and the audit writer pool. Construct
once at process root and pass explicitly — never via package-level singletons.
*/
type Runtime struct {
	Quotes *broker.QuoteCache
	Stress *broker.StressCache
	Rules  *broker.InstrumentRulesCache
	Audit  *audit.WriterPool
}

/*
New wires independent broker caches for one execution context (live engine or test).
*/
func New(ctx context.Context, pool *qpool.Q[any]) (*Runtime, error) {
	if pool == nil {
		return nil, fmt.Errorf("runtime: pool is required")
	}

	quotes := broker.NewQuoteCache(ctx, pool)
	quotes.Start(pool)

	stress := broker.NewStressCache(ctx, pool)
	stress.Start(pool)

	rules := broker.NewInstrumentRulesCache(ctx)
	rules.Start(pool)

	return &Runtime{
		Quotes: quotes,
		Stress: stress,
		Rules:  rules,
		Audit:  audit.NewWriterPool(),
	}, nil
}

/*
OpenAudit returns the shared audit writer for the configured path, refcounted per pool.
*/
func (runtime *Runtime) OpenAudit() (*audit.Writer, error) {
	if runtime == nil || runtime.Audit == nil {
		return nil, nil
	}

	return runtime.Audit.OpenConfigured()
}
