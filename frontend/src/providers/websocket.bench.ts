import { bench, describe } from "vitest";
import { paintKernelList } from "#/components/terminal/kernel-list";
import type { Measurement } from "#/types/measurement";

const row = (raw: number): Measurement => ({
	source: "pumpdump",
	metric: "raw",
	symbol: "BTC/USD",
	at: new Date().toISOString(),
	raw,
	normalized: raw,
	uncertainty: null,
	validity: { state: "valid", readiness: "ready" },
	scale: { kind: "unit", from: "0", through: "1" },
});

describe("DRAW paint", () => {
	bench("paints a measurements delta", () => {
		paintKernelList([row(Math.random())], "BTC/USD");
	});
});
