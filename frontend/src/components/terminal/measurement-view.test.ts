import { describe, expect, it } from "vitest";
import type { Measurement } from "#/types/measurement";
import {
	dedupeEpoch,
	latestEpoch,
	measurementIdentity,
	orderedEpoch,
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
});
