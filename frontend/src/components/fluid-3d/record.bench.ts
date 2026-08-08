import { bench, describe } from "vitest";
import { FluidRecordReader } from "./record";

const payload = new Uint8Array(16 * 1024);
const header = new Uint8Array(8);
header.set([0x53, 0x46, 0x44, 0x31]);
new DataView(header.buffer).setUint32(4, payload.byteLength, true);

describe("FluidRecordReader", () => {
	bench("reassembles one SCTP-safe segment", () => {
		const reader = new FluidRecordReader();
		reader.push(header.buffer);
		reader.push(payload.buffer);
	});
});
