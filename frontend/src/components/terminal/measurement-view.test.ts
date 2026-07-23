import { describe, expect, it } from "vitest";
import type { Measurement } from "#/types/measurement";
import {
	dedupeEpoch,
	headlineMetric,
	headlineReading,
	latestByMetric,
	latestEpoch,
	measurementIdentity,
	metricLabel,
	orderedEpoch,
	resolveKernelStatus,
	sideLabel,
} from "./measurement-view";

const measurement = (
	metric: string,
	at: string,
	raw: number,
	side = "",
): Measurement => ({
	source: "exhaustion",
	metric,
	symbol: "BTC/USD",
	side,
	at,
	raw,
	normalized: raw,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: at, through: at },
});

describe("orderedEpoch", () => {
	it("returns every metric from the latest observation tick", () => {
		const at = "2026-07-14T16:25:24Z";
		const values = [
			measurement("mechanical", at, 0.18),
			measurement("thermal", at, 0),
			measurement("fragile", at, 0),
			measurement("reversal", at, 0),
			measurement("urgency", at, 0.42),
			measurement("strength", at, 0.07),
			measurement("value", at, 0.11),
		];

		expect(latestEpoch(values)).toHaveLength(7);
		expect(orderedEpoch(values, "strength")).toHaveLength(7);
		expect(orderedEpoch(values, "strength")[0]?.metric).toBe("strength");
		expect(headlineMetric("hawkes")).toBe("conditional_intensity");
		expect(headlineMetric("toxicity")).toBe("touch_quantity");
		expect(headlineMetric("liquidity")).toBe("scarcity_score");
		const bid = measurement("conditional_intensity", at, 0.18, "buy");
		const ask = measurement("conditional_intensity", at, 0.21, "sell");
		expect(latestByMetric([bid, ask], "conditional_intensity")).toBe(ask);
		expect(
			headlineReading([measurement("event_count", at, 4, "buy")], "hawkes")
				?.metric,
		).toBe("event_count");
		expect(
			headlineReading(
				[
					measurement("event_count", at, 4, "buy"),
					measurement("arrival_rate", at, 0.5, "buy"),
				],
				"hawkes",
			)?.metric,
		).toBe("arrival_rate");
	});

	it("dedupes duplicate metric identities within one observation tick", () => {
		const at = "2026-07-14T16:25:24Z";
		const values = [
			measurement("value", at, 0.11),
			measurement("value", at, 0.22),
			measurement("thin_score", at, 0.31),
			measurement("thin_score", at, 0.44),
		];

		expect(dedupeEpoch(latestEpoch(values))).toHaveLength(2);
		expect(
			dedupeEpoch(latestEpoch(values)).find(
				(measurement) => measurement.metric === "value",
			)?.raw,
		).toBe(0.22);
	});

	it("keeps distinct subjects separate within one observation tick", () => {
		const at = "2026-07-14T16:25:24Z";
		const left = measurement("value", at, 0.11);
		const right = {
			...measurement("value", at, 0.22),
			subject: "ETH/USD",
		};

		expect(measurementIdentity(left)).not.toBe(measurementIdentity(right));
		expect(dedupeEpoch([left, right])).toHaveLength(2);
	});

	it("does not collide when field values contain delimiter characters", () => {
		const at = "2026-07-14T16:25:24Z";
		const left = {
			...measurement("value", at, 0.11),
			side: "buy:sell",
		};
		const right = {
			...measurement("value", at, 0.22),
			side: "buy",
			subject: "sell",
		};

		expect(measurementIdentity(left)).not.toBe(measurementIdentity(right));
		expect(dedupeEpoch([left, right])).toHaveLength(2);
	});

	it("sorts metrics deterministically after the headline", () => {
		const at = "2026-07-14T16:25:24Z";
		const values = [
			measurement("sync", at, 0.2),
			measurement("strength", at, 0.1),
			measurement("correlation", at, 0.3),
			measurement("decoupled", at, 0.4),
		];

		expect(
			orderedEpoch(values, "strength").map((entry) => entry.metric),
		).toEqual(["strength", "correlation", "decoupled", "sync"]);
	});

	it("humanizes semantic liquidity metric identifiers", () => {
		expect(metricLabel("scarcity_score")).toBe("Scarcity Score");
		expect(metricLabel("relative_touch_depth")).toBe("Relative Touch Depth");
		expect(metricLabel("reported_volume_notional")).toBe(
			"Reported Volume Notional",
		);
	});

	it("labels hawkes cross-kernel sides instead of blank duplicates", () => {
		expect(sideLabel("buy")).toBe("Bid");
		expect(sideLabel("sell")).toBe("Ask");
		expect(sideLabel("buy_to_buy")).toBe("Buy→Buy");
		expect(sideLabel("sell_to_buy")).toBe("Sell→Buy");
		expect(sideLabel("buy_to_sell")).toBe("Buy→Sell");
		expect(sideLabel("sell_to_sell")).toBe("Sell→Sell");
	});
});

describe("resolveKernelStatus", () => {
	it("labels a quiet focus as off-focus when the universe has the source", () => {
		expect(resolveKernelStatus(undefined, true)).toBe("unfocused");
		expect(resolveKernelStatus(undefined, false)).toBe("waiting");
		expect(
			resolveKernelStatus(
				measurement("strength", "2026-07-14T16:25:24Z", 0.2),
				true,
			),
		).toBe("measured");
	});
});

describe("compact buffer rows", () => {
	it("reads headline strength from a metrics map on the store row", () => {
		const at = "2026-07-19T04:00:00Z";
		const row: Measurement = {
			source: "correlation",
			symbol: "BTC/USD",
			at,
			raw: 0,
			normalized: null,
			uncertainty: null,
			validity: { state: "valid", readiness: "observation" },
			scale: { kind: "observation_window", from: at, through: at },
			metrics: { strength: 0.4, herd_score: 0.2 },
		};

		expect(headlineReading([row], "correlation")?.raw).toBe(0.4);
		expect(latestByMetric([row], "herd_score")?.raw).toBe(0.2);
		expect(resolveKernelStatus(headlineReading([row], "correlation"), true)).toBe(
			"measured",
		);
	});
});
