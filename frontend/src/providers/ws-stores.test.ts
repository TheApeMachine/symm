import { describe, expect, it, vi } from "vitest";
import {
	paintRegistered,
	registerPainter,
} from "#/providers/ws-stores";

describe("ws-stores", () => {
	it("registers painters under backend wire keys and dispatches updates", () => {
		const paint = vi.fn();
		const unregister = registerPainter("measurements", paint);

		paintRegistered("measurements", { source: "hawkes" });

		expect(paint).toHaveBeenCalledTimes(1);
		expect(paint).toHaveBeenCalledWith({ source: "hawkes" });

		unregister();
		paintRegistered("measurements", { source: "sentiment" });

		expect(paint).toHaveBeenCalledTimes(1);
	});
});
