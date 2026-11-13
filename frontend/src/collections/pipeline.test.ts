import { describe, expect, it } from "vitest";
import {
	formatNanos,
	healthOf,
	peakEma,
	type PipelineNode,
} from "./pipeline";

const node = (partial: Partial<PipelineNode>): PipelineNode => ({
	id: "cvd",
	group: "signal",
	n: 10,
	last_ns: 1_000,
	ema_ns: 1_000,
	seen_at: Date.now() * 1_000_000,
	status: "ok",
	...partial,
});

describe("pipeline health", () => {
	it("marks missing and empty nodes idle", () => {
		expect(healthOf(undefined, 0)).toBe("idle");
		expect(healthOf(node({ n: 0 }), 0)).toBe("idle");
	});

	it("marks error status red regardless of latency", () => {
		expect(healthOf(node({ status: "error", ema_ns: 100 }), 0)).toBe("error");
	});

	it("keeps cheap work healthy even when it is the peak", () => {
		const cheap = node({ ema_ns: 50_000 });
		expect(healthOf(cheap, peakEma([cheap]))).toBe("healthy");
	});

	it("escalates relative pressure above a millisecond", () => {
		const slow = node({ ema_ns: 5_000_000 });
		expect(healthOf(slow, 5_000_000)).toBe("high");
	});
});

describe("formatNanos", () => {
	it("picks a compact unit", () => {
		expect(formatNanos(500)).toBe("500ns");
		expect(formatNanos(2_400)).toBe("2.4µs");
		expect(formatNanos(12_340_000)).toBe("12.34ms");
	});
});
