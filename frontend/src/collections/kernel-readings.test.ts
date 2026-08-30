import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { addMeasurement, getKernelReadingStore } from "#/collections/app";
import { EnvelopeMeasurement } from "#/providers/telemetry/telemetry/envelope-measurement";

/*
measurement builds a real EnvelopeMeasurement the way the wire delivers one, so
these tests exercise the same snrDefined() gate the dispatcher reads rather than
a stub that could drift from it.
*/
const measurement = (snr: number, snrDefined: boolean): EnvelopeMeasurement => {
	const builder = new flatbuffers.Builder(0);

	EnvelopeMeasurement.startEnvelopeMeasurement(builder);
	EnvelopeMeasurement.addSnr(builder, snr);
	EnvelopeMeasurement.addSnrDefined(builder, snrDefined);
	const offset = EnvelopeMeasurement.endEnvelopeMeasurement(builder);

	builder.finish(offset);

	return EnvelopeMeasurement.getRootAsEnvelopeMeasurement(
		new flatbuffers.ByteBuffer(builder.asUint8Array()),
	);
};

const readingsOf = (source: string): number[] => {
	const ring = getKernelReadingStore(source).state;
	const values: number[] = [];

	for (let i = 0; i < ring.getBufferLength(); i++) {
		const value = ring.get(i);
		if (value !== undefined) values.push(value);
	}

	return values;
};

describe("kernel readings", () => {
	it("collects a reading when the backend defines an SNR", () => {
		addMeasurement("readings-defined", measurement(4.5, true));

		expect(readingsOf("readings-defined")).toEqual([4.5]);
	});

	it("ignores a row whose SNR is undefined rather than reading it as zero", () => {
		addMeasurement("readings-undefined", measurement(0, false));

		expect(readingsOf("readings-undefined")).toEqual([]);
	});

	/*
		The regression this guards: measurement rings hold every row, defined or
		not, so a run of SNR-less rows longer than the ring used to evict every
		real reading out of it — the kernel fell back to Standby with an empty
		blue trace despite nothing having gone wrong. Sparse updates must leave
		the history exactly as it was.
	*/
	it("keeps its readings through a long run of empty updates", () => {
		addMeasurement("readings-sparse", measurement(7.25, true));

		for (let i = 0; i < 200; i++) {
			addMeasurement("readings-sparse", measurement(0, false));
		}

		expect(readingsOf("readings-sparse")).toEqual([7.25]);
	});

	it("advances only on real readings, preserving their order", () => {
		addMeasurement("readings-order", measurement(1, true));
		addMeasurement("readings-order", measurement(0, false));
		addMeasurement("readings-order", measurement(2, true));
		addMeasurement("readings-order", measurement(0, false));
		addMeasurement("readings-order", measurement(3, true));

		expect(readingsOf("readings-order")).toEqual([1, 2, 3]);
	});
});
