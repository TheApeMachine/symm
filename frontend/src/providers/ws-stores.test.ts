import { describe, expect, it } from "vitest";
import { drawers } from "#/providers/ws-stores";

describe("drawers", () => {
	it("registers paint under backend wire keys", () => {
		expect(typeof drawers.measurements.paint).toBe("function");
		expect(drawers.measurements.keys?.hawkes.input).toBe("history");
		expect(drawers.measurements.keys?.health.input).toBe("latest");
		expect(drawers.measurements.keys?.palette).toBeTypeOf("function");
		expect(typeof drawers.tick.paint).toBe("function");
		expect(typeof drawers.decisions.paint).toBe("function");
		expect(drawers.resonance.paint.input).toBe("latest");
		expect(drawers.manifold.paint.input).toBe("latest");
		expect(typeof drawers.manifold_particles.paint).toBe("function");
		expect(typeof drawers.manifold_wave.paint).toBe("function");
		expect(drawers.cognition.paint.input).toBe("latest");
		expect(drawers.resonance.keys?.prediction.input).toBe("history");
		expect(drawers.causal.keys?.allocation.input).toBe("latest");
		expect(drawers.manifold.keys?.allocation.input).toBe("latest");
		expect(drawers.resonance.keys?.allocation.input).toBe("latest");
	});
});
