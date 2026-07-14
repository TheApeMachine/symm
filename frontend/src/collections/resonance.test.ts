import { beforeEach, describe, expect, it } from "vitest";
import { resonanceStore } from "./resonance";

describe("resonanceStore", () => {
	beforeEach(() => {
		resonanceStore.setState(() => ({
			resonance: {},
			version: 0,
		}));
	});

	it("accepts the singleton domain object published by the backend", () => {
		resonanceStore.actions.updateFrame({
			source: "resonance",
			symbol: "RESONANCE/TEST",
			at: "2026-07-12T00:00:00Z",
			samples: 1,
		});

		expect(
			resonanceStore.state.resonance["RESONANCE/TEST"]?.values().at(-1),
		).toMatchObject({ source: "resonance", samples: 1 });
		expect(resonanceStore.state.version).toBe(1);
	});

	it("retains every publish frame even when the observation timestamp is unchanged", () => {
		const frame = (at: string, samples: number) => ({
			source: "resonance",
			symbol: "BTC/USD",
			at,
			samples,
			surprise: 0.25,
			layers: [{ state: [samples], prediction: [samples + 0.5] }],
		});

		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:00Z", 1));
		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:00Z", 2));
		resonanceStore.actions.updateFrame(frame("2026-07-12T00:00:01Z", 3));

		expect(resonanceStore.state.resonance["BTC/USD"]?.values()).toEqual([
			frame("2026-07-12T00:00:00Z", 1),
			frame("2026-07-12T00:00:00Z", 2),
			frame("2026-07-12T00:00:01Z", 3),
		]);
	});
});
