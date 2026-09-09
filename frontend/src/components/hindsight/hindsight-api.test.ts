import { afterEach, describe, expect, it, vi } from "vitest";
import {
	fetchHindsightRuns,
	fetchHindsightCaptures,
	fetchHindsightStates,
	fetchHindsightState,
	fetchHindsightEnvelope,
	fetchHindsightGaps,
	fetchHindsightLifecycle,
	fetchHindsightTimeline,
	fetchHindsightResident,
	fetchHindsightMetricMap,
} from "./hindsight-api";

afterEach(() => vi.unstubAllGlobals());

const respond = (status: number, body: unknown) => {
	vi.stubGlobal("window", {
		location: { protocol: "http:", hostname: "localhost" },
	});
	vi.stubGlobal(
		"fetch",
		vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })),
	);
};

describe("Hindsight archive reads", () => {
	it("preserves the run identity and timestamp used by run selection", async () => {
		const run = { id: "run", startedAt: "2026-09-09T00:00:00Z", positions: 2 };
		respond(200, [run]);
		expect(await fetchHindsightRuns()).toEqual([run]);
	});

	it("rejects storage-row JSON instead of rendering invalid dates and unselectable runs", async () => {
		respond(200, [{ ID: "run", StartedAt: "2026-09-09T00:00:00Z" }]);
		await expect(fetchHindsightRuns()).rejects.toThrow("invalid run metadata");
	});

	it.each([
		["runs", () => fetchHindsightRuns()],
		["captures", () => fetchHindsightCaptures("run")],
		["states", () => fetchHindsightStates("run")],
		["state", () => fetchHindsightState("run", 2, 1)],
		["envelope", () => fetchHindsightEnvelope("run", 2)],
		["gaps", () => fetchHindsightGaps("run")],
		["lifecycle", () => fetchHindsightLifecycle("run")],
		["timeline", () => fetchHindsightTimeline({ run: "run" })],
		["resident", () => fetchHindsightResident("run", "BTC/USD", 2, 1)],
		["semantics", () => fetchHindsightMetricMap()],
	])("surfaces a failed %s read instead of returning an empty archive", async (_name, read) => {
		respond(503, "archive unavailable");
		await expect(read()).rejects.toThrow("503");
	});

	it("keeps a genuinely absent exact artifact distinct from a failed read", async () => {
		respond(404, "not found");
		expect(await fetchHindsightState("run", 2, 1)).toBeNull();
		expect(await fetchHindsightEnvelope("run", 2)).toBeNull();
	});
});
