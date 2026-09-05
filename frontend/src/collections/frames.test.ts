import { describe, expect, it } from "vitest";
import { latestOf } from "./circular";
import { createKeyedStore } from "./store";

describe("createKeyedStore via frames regression", () => {
	it("retains bounded history under one key", () => {
		const store = createKeyedStore<{ n: number }>()("tick", 3, () => "current");

		store.actions.updateFrame([{ n: 1 }, { n: 2 }, { n: 3 }, { n: 4 }]);

		expect(latestOf(store.state.tick.current)?.n).toBe(4);
		expect(store.state.tick.current?.values()).toEqual([
			{ n: 2 },
			{ n: 3 },
			{ n: 4 },
		]);
	});
});
