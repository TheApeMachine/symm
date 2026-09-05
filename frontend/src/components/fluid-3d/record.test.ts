import { describe, expect, it } from "vitest";
import { FluidRecordReader } from "./record";

const CHUNK_HEADER_BYTES = 16;
const MAGIC = [0x53, 0x46, 0x44, 0x31];

const chunk = (
	frameId: number,
	chunkIndex: number,
	chunkCount: number,
	payload: Uint8Array,
) => {
	const message = new Uint8Array(CHUNK_HEADER_BYTES + payload.byteLength);
	message.set(MAGIC, 0);
	const view = new DataView(message.buffer);
	view.setUint32(4, frameId, true);
	view.setUint32(8, chunkIndex, true);
	view.setUint32(12, chunkCount, true);
	message.set(payload, CHUNK_HEADER_BYTES);
	return message.buffer;
};

describe("FluidRecordReader.push", () => {
	it("reassembles chunks into the original bytes regardless of arrival order", () => {
		const source = new TextEncoder().encode(
			'{"fields":{"Density":[0.25,0.5,1]}}',
		);
		const reader = new FluidRecordReader();
		const one = source.slice(0, 9);
		const two = source.slice(9);

		expect(reader.push(chunk(1, 1, 2, two))).toBeNull();
		expect(reader.push(chunk(1, 0, 2, one))).toEqual(source.buffer);
	});

	it("discards a chunk from an obsolete frame", () => {
		const reader = new FluidRecordReader();
		const part = new TextEncoder().encode("drop me");

		reader.push(chunk(5, 0, 1, part));
		expect(reader.push(chunk(3, 0, 1, part))).toBeNull();
	});

	it("ignores a duplicate chunk", () => {
		const reader = new FluidRecordReader();
		const one = new TextEncoder().encode("same");
		const two = new TextEncoder().encode("same");

		reader.push(chunk(1, 0, 2, one));
		expect(reader.push(chunk(1, 0, 2, two))).toBeNull();
	});

	it("rejects a chunk shorter than its header", () => {
		const reader = new FluidRecordReader();

		expect(() => reader.push(new Uint8Array([1, 2, 3]).buffer)).toThrow(
			"shorter than its 16-byte header",
		);
	});
});
