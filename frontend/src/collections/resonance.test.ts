import { describe, expect, it } from "vitest";
import { resonanceStore } from "./resonance";

describe("resonanceStore", () => {
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
	});
});
