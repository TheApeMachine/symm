import { describe, expect, it } from "vitest";
import {
	clearManifoldBinary,
	latestLattice,
	parseManifoldBinary,
	retainManifoldBinary,
} from "#/providers/manifold-binary";

const encodeFixture = (): ArrayBuffer => {
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

describe("parseManifoldBinary", () => {
	it("decodes SMF1 uint16 lattices into float samples", () => {
		const plane = parseManifoldBinary(encodeFixture());

		expect(plane).not.toBeNull();
		expect(plane?.kind).toBe("rho");
		expect(plane?.symbol).toBe("BTC/USD");
		expect(plane?.width).toBe(2);
		expect(plane?.height).toBe(2);
		expect(plane?.samples[0]).toBeCloseTo(0);
		expect(plane?.samples[1]).toBeCloseTo(1);
		expect(plane?.samples[2]).toBeCloseTo(0.5, 2);
	});

	it("retains the latest plane by kind", () => {
		clearManifoldBinary();
		expect(retainManifoldBinary(encodeFixture())).toBe("rho");
		expect(latestLattice("rho")?.symbol).toBe("BTC/USD");
		clearManifoldBinary();
		expect(latestLattice("rho")).toBeNull();
	});
});
