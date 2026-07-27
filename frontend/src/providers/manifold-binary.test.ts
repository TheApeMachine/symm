import { describe, expect, it } from "vitest";
import {
	clearManifoldBinary,
	latestDisplay,
	parseManifoldBinary,
	retainManifoldBinary,
} from "#/providers/manifold-binary";

const encodeLatticeFixture = (): ArrayBuffer => {
	const symbol = "BTC/USD";
	const width = 2;
	const height = 2;
	const samples = new Uint16Array([0, 65535, 32768, 16384]);
	const header = 26 + symbol.length;
	const buffer = new ArrayBuffer(header + samples.byteLength);
	const view = new DataView(buffer);
	const bytes = new Uint8Array(buffer);

	bytes[0] = 0x53;
	bytes[1] = 0x4d;
	bytes[2] = 0x46;
	bytes[3] = 0x31;
	bytes[4] = 1;
	view.setUint16(5, width, true);
	view.setUint16(7, height, true);
	view.setFloat32(9, 0, true);
	view.setFloat32(13, 1, true);
	view.setBigUint64(17, 0n, true);
	bytes[25] = symbol.length;
	new TextEncoder().encodeInto(symbol, bytes.subarray(26));
	bytes.set(new Uint8Array(samples.buffer), header);
	return buffer;
};

const encodeDisplayFixture = (): ArrayBuffer => {
	const symbol = "BTC/USD";
	const width = 2;
	const height = 2;
	const rgba = new Uint8Array([
		10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 255, 255, 255, 255,
	]);
	const header = 26 + symbol.length;
	const buffer = new ArrayBuffer(header + rgba.byteLength);
	const view = new DataView(buffer);
	const bytes = new Uint8Array(buffer);

	bytes[0] = 0x53;
	bytes[1] = 0x4d;
	bytes[2] = 0x46;
	bytes[3] = 0x31;
	bytes[4] = 5;
	view.setUint16(5, width, true);
	view.setUint16(7, height, true);
	view.setFloat32(9, 0, true);
	view.setFloat32(13, 1, true);
	view.setBigUint64(17, 0n, true);
	bytes[25] = symbol.length;
	new TextEncoder().encodeInto(symbol, bytes.subarray(26));
	bytes.set(rgba, header);
	return buffer;
};

describe("parseManifoldBinary", () => {
	it("rejects legacy scalar lattice frames", () => {
		const plane = parseManifoldBinary(encodeLatticeFixture());

		expect(plane).toBeNull();
	});

	it("decodes a backend-composited RGBA display frame", () => {
		clearManifoldBinary();
		expect(retainManifoldBinary(encodeDisplayFixture())).toBe("display");
		const frame = latestDisplay();
		expect(frame?.width).toBe(2);
		expect(frame?.height).toBe(2);
		expect(frame?.rgba[0]).toBe(10);
		expect(frame?.rgba[12]).toBe(255);
		clearManifoldBinary();
		expect(latestDisplay()).toBeNull();
	});

	it("does not retain scalar planes", () => {
		clearManifoldBinary();
		expect(retainManifoldBinary(encodeLatticeFixture())).toBeNull();
		expect(latestDisplay()).toBeNull();
	});
});
