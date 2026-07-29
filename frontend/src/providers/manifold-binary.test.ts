import { describe, expect, it } from "vitest";
import {
	clearManifoldBinary,
	latestDisplay,
	retainManifoldBinary,
	retainManifoldMeta,
} from "#/providers/manifold-binary";

	const encodeDisplayFixture = (): ArrayBuffer => {
	const rgba = new Uint8Array([
		10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 255, 255, 255, 255,
	]);
	return rgba.buffer.slice(0);
};

describe("retainManifoldBinary", () => {
	it("retains one backend-composited RGBA display frame once meta arrived", () => {
		clearManifoldBinary();
		retainManifoldMeta({ symbol: "BTC/USD", width: 2, height: 2 });
		expect(retainManifoldBinary(encodeDisplayFixture())).toBe("display");
		const frame = latestDisplay();
		expect(frame?.width).toBe(2);
		expect(frame?.height).toBe(2);
		expect(frame?.rgba[0]).toBe(10);
		expect(frame?.rgba[12]).toBe(255);
		clearManifoldBinary();
		expect(latestDisplay()).toBeNull();
	});

	it("rejects raw buffers before manifold meta arrives", () => {
		clearManifoldBinary();
		expect(retainManifoldBinary(encodeDisplayFixture())).toBeNull();
		expect(latestDisplay()).toBeNull();
	});

	it("rejects raw buffers whose size does not match retained dimensions", () => {
		clearManifoldBinary();
		retainManifoldMeta({ symbol: "BTC/USD", width: 4, height: 4 });
		expect(retainManifoldBinary(encodeDisplayFixture())).toBeNull();
		expect(latestDisplay()).toBeNull();
	});
});
