import { describe, expect, it } from "vitest";
import { terminalStore } from "./terminal";

describe("cycleFieldLayer", () => {
	it("cycles through composite, coherence, and gas inspection", () => {
		expect(terminalStore.state.fieldLayer).toBe("Composite");

		terminalStore.actions.cycleFieldLayer();
		expect(terminalStore.state.fieldLayer).toBe("Coherence");

		terminalStore.actions.cycleFieldLayer();
		expect(terminalStore.state.fieldLayer).toBe("Gas");

		terminalStore.actions.cycleFieldLayer();
		expect(terminalStore.state.fieldLayer).toBe("Composite");
	});
});
