import { describe, expect, it } from "vitest";
import {
	clearManifoldBinary,
	latestDisplay,
	parseManifoldBinary,
	retainManifoldBinary,
} from "#/providers/manifold-binary";

const displayFixture = (): ArrayBuffer => {
	const symbol = "BTC/USD";
	const rgba = new Uint8Array([
		10, 20, 30, 255,
		40, 50, 60, 255,
		70, 80, 90, 255,
		100, 110, 120, 255,
	]);
	const pixelOffset = 26 + symbol.length;
	const buffer = new ArrayBuffer(pixelOffset + rgba.byteLength);
	const view = new DataView(buffer);
	const bytes = new Uint8Array(buffer);

	bytes.set(new TextEncoder().encode("SMF1"));
	bytes[4] = 5;
	view.setUint16(5, 2, true);
	view.setUint16(7, 2, true);
	view.setFloat32(9, 0, true);
	view.setFloat32(13, 1, true);
	view.setBigUint64(17, 0n, true);
	bytes[25] = symbol.length;
	bytes.set(new TextEncoder().encode(symbol), 26);
	bytes.set(rgba, pixelOffset);

	return buffer;
};

describe("parseManifoldBinary", () => {
	it("decodes and retains a backend GPU display", () => {
		clearManifoldBinary();
		const buffer = displayFixture();
		const parsed = parseManifoldBinary(buffer);

		expect(parsed?.symbol).toBe("BTC/USD");
		expect(parsed?.width).toBe(2);
		expect(parsed?.height).toBe(2);
		expect(Array.from(parsed?.rgba ?? [])).toEqual([
			10, 20, 30, 255,
			40, 50, 60, 255,
			70, 80, 90, 255,
			100, 110, 120, 255,
		]);
		expect(retainManifoldBinary(buffer)).toBe(true);
		expect(latestDisplay()?.symbol).toBe("BTC/USD");
	});

	it("rejects raw RGBA bytes without an SMF1 envelope", () => {
		expect(parseManifoldBinary(new ArrayBuffer(64 * 64 * 4))).toBeNull();
	});

	it("rejects a truncated display payload", () => {
		const buffer = displayFixture().slice(0, -1);

		expect(parseManifoldBinary(buffer)).toBeNull();
	});
});