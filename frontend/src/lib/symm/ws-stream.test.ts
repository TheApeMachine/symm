import { describe, expect, it } from "vitest";

import { AuditDataProvider } from "#/components/symm/audit-data-provider";
import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { OhlcDataProvider } from "#/components/symm/ohlc-data-provider";
import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";
import { TradesDataProvider } from "#/components/symm/trades-data-provider";
import { defaultSymmTelemetryStores } from "#/lib/symm/telemetry-stores";
import { routePayload } from "#/lib/symm/ws-stream";
import { TickStore } from "#/lib/symm/tick-store";

describe("routePayload", () => {
	it("increments tick count from crypto tick events", () => {
		TickStore.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "tick",
			ts: "2026-05-28T01:10:10Z",
		});
		routePayload(defaultSymmTelemetryStores, {
			event: "tick",
			ts: "2026-05-28T01:10:11Z",
		});

		expect(TickStore.snapshot()).toBe(2);
		TickStore.reset();
	});

	it("does not reset tick count on hello frames", () => {
		TickStore.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "tick",
			ts: "2026-05-28T01:10:10Z",
		});
		routePayload(defaultSymmTelemetryStores, {
			event: "hello",
			ts: "2026-05-28T01:10:11Z",
		});

		expect(TickStore.snapshot()).toBe(1);
		TickStore.reset();
	});

	it("updates tick count from heartbeat proof of life events", () => {
		TickStore.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "heartbeat",
			ts: "2026-05-28T01:10:10Z",
			seq: 42,
			throttled: true,
			queue_depth: 4,
			dropped: 2,
		});

		expect(TickStore.snapshot()).toBe(42);
		expect(TickStore.statusSnapshot()).toEqual({
			seq: 42,
			throttled: true,
			queueDepth: 4,
			dropped: 2,
		});
		TickStore.reset();
	});

	it("routes audit events into the audit panel provider", () => {
		AuditDataProvider.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "audit",
			audit_event: "trade_entry_fill",
			seq: 8,
			ts: "2026-05-28T01:10:10Z",
			symbol: "BTC/EUR",
			slot_eur: 10,
			confidence: 0.9,
		});

		expect(AuditDataProvider.snapshot()[0]).toMatchObject({
			seq: 8,
			event: "trade entry fill",
			symbol: "BTC/EUR",
		});
		AuditDataProvider.reset();
	});

	it("routes mark events to trades without mutating candle history", () => {
		TradesDataProvider.reset();
		const bars: number[] = [];
		const unregister = OhlcDataProvider.registerSymbol("ROUTE/EUR", (bar) => {
			bars.push(bar.sec);
		});

		routePayload(defaultSymmTelemetryStores, {
			Type: "paper",
			Currency: "EUR",
			Balance: 198.9,
			Inventory: { ROUTE: 10 },
			AvgEntry: { ROUTE: 0.4 },
			Marks: { "ROUTE/EUR": 0.4 },
		});

		routePayload(defaultSymmTelemetryStores, {
			event: "mark",
			ts: "2026-05-28T01:10:10Z",
			symbol: "ROUTE/EUR",
			price: 0.42,
		});

		expect(bars).toEqual([]);
		expect(TradesDataProvider.snapshot()[0]?.markPrice).toBe(0.42);
		TradesDataProvider.reset();
		unregister();
	});

	it("routes engine pulse events to the prediction provider", () => {
		PredictionsDataProvider.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "engine_pulse",
			seq: 7,
			phase: "scan",
			measurements: 3,
			open: 0,
			ts: "2026-05-28T01:10:10Z",
			avg_prediction: 0.004,
			avg_error: 0.001,
		});

		expect(PredictionsDataProvider.snapshot()).toMatchObject({
			event: "engine_pulse",
			seq: 7,
			avg_prediction: 0.004,
		});
		PredictionsDataProvider.reset();
	});

	it("hydrates confidence gauges from wallet frames", () => {
		ConfidenceDataProvider.reset();

		routePayload(defaultSymmTelemetryStores, {
			event: "wallet",
			Currency: "EUR",
			Balance: 198.9,
			Inventory: {},
			AvgEntry: {},
			Marks: {},
			gauge_confidence: {
				hawkes: 0.33,
			},
		});

		expect(ConfidenceDataProvider.snapshot().get("hawkes")?.confidence).toBe(
			0.33,
		);
		ConfidenceDataProvider.reset();
	});
});
