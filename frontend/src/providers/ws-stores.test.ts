import { describe, expect, it } from "vitest";
import { drawers } from "#/providers/ws-stores";

describe("drawers", () => {
	it("registers paint under backend wire keys", () => {
		expect(typeof drawers.measurements.paint).toBe("function");
		expect(drawers.measurements.keys?.hawkes.input).toBe("history");
		expect(drawers.measurements.keys?.palette).toBeTypeOf("function");
		expect(typeof drawers.tick.paint).toBe("function");
		expect(typeof drawers.decisions.paint).toBe("function");
		expect(typeof drawers.resonance.paint).toBe("function");
		expect(typeof drawers.manifold.paint).toBe("function");
		expect(typeof drawers.cognition.paint).toBe("function");
		expect(drawers.causal.keys?.allocation.input).toBe("history");
		expect(drawers.manifold.keys?.allocation.input).toBe("history");
		expect(drawers.resonance.keys?.allocation.input).toBe("history");
	});
});
