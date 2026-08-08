import { describe, expect, it } from "vitest";
import { FluidRecordReader } from "./record";

const header = (length: number) => {
	const value = new Uint8Array(8);
	value.set([0x53, 0x46, 0x44, 0x31]);
	new DataView(value.buffer).setUint32(4, length, true);
	return value.buffer;
};

describe("FluidRecordReader.push", () => {
	it("reassembles ordered segments into the original bytes", () => {
		const source = new TextEncoder().encode(
			'{"fields":{"Density":[0.25,0.5,1]}}',
		);
		const reader = new FluidRecordReader();

		expect(reader.push(header(source.byteLength))).toBeNull();
		expect(reader.push(source.slice(0, 9).buffer)).toBeNull();
		expect(new Uint8Array(reader.push(source.slice(9).buffer) ?? [])).toEqual(
			source,
		);
	});

	it("rejects a segment beyond the declared record length", () => {
		const reader = new FluidRecordReader();
		reader.push(header(2));

		expect(() => reader.push(new Uint8Array([1, 2, 3]).buffer)).toThrow(
			"exceeded its declared length",
		);
	});
});
