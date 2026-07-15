import { describe, expect, it } from "vitest";
import { type ManifoldFrame, manifoldStore } from "./manifold";

describe("manifoldStore", () => {
	it("accepts the singleton domain object published by the backend", () => {
		manifoldStore.actions.reset();
		manifoldStore.actions.updateFrame({
			source: "manifold",
			symbol: "MANIFOLD/TEST",
			at: "2026-07-12T00:00:00Z",
			epoch: 1,
		});

		expect(
			manifoldStore.state.manifold["MANIFOLD/TEST"]?.values().at(-1),
		).toMatchObject({ source: "manifold", epoch: 1 });
	});

	it("ignores manifold frames without a symbol", () => {
		manifoldStore.actions.reset();
		manifoldStore.actions.updateFrame({
			source: "manifold",
			symbol: "",
			at: "2026-07-12T00:00:00Z",
			epoch: 1,
		} as ManifoldFrame);

		expect(Object.keys(manifoldStore.state.manifold)).toHaveLength(0);
	});
});
