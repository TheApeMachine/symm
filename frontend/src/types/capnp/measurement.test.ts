import { describe, expect, it } from "vitest";
import { readWireMeasurement } from "./measurement";

describe("reading a measurement off the wire", () => {
	it("says how big a frame was when it is too short to be a message", () => {
		// The reader used to run off the end of the segment table and fail
		// as a missing `getWord` deep inside the capnp library, which named
		// neither the frame nor its size. A peer sending something else on
		// this socket is the likely cause, so the size is the thing worth
		// reporting.
		expect(() => readWireMeasurement(new Uint8Array(0))).toThrow(
			/0 bytes, too short/,
		);

		expect(() => readWireMeasurement(new Uint8Array(4))).toThrow(
			/4 bytes, too short/,
		);
	});

	it("refuses a frame that is long enough but carries nothing readable", () => {
		expect(() => readWireMeasurement(new Uint8Array(64))).toThrow();
	});
});
