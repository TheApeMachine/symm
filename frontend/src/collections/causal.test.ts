import { describe, expect, it } from "vitest";
import { causalStore } from "./causal";

describe("causalStore", () => {
	it("accepts the singleton domain object published by the backend", () => {
		causalStore.actions.updateFrame({
			source: "causal",
			symbol: "CAUSAL/TEST",
			at: "2026-07-12T00:00:00Z",
			ready: false,
		});

		expect(
			causalStore.state.causal["CAUSAL/TEST"]?.values().at(-1),
		).toMatchObject({ source: "causal", ready: false });
	});
});
