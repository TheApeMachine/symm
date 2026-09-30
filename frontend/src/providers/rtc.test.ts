import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { signals, symbolsAtom } from "#/collections/app";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrameT } from "#/providers/telemetry/telemetry/measurements-frame";
import { MessageT } from "#/providers/telemetry/telemetry/message";
import { dispatchMeasurementsBuffer } from "./rtc";

describe("rtc dispatchMeasurementsBuffer", () => {
	it("dispatches MeasurementsFrame inside a Message to the signals store and tracks symbols", () => {
		const m = new MeasurementT();
		m.source = "training";
		m.symbol = "BTC/USD";
		m.at = 100n;
		m.tick = 10n;

		const frame = new MeasurementsFrameT([m]);
		const msg = new MessageT(1n, Frame.MeasurementsFrame, frame);

		const builder = new flatbuffers.Builder(1024);
		const offset = msg.pack(builder);
		builder.finish(offset, "SYMM");

		const buffer = new flatbuffers.ByteBuffer(builder.asUint8Array());
		dispatchMeasurementsBuffer(buffer);

		expect(signals.training.state["BTC/USD"]).toBeDefined();
		expect(signals.training.state["BTC/USD"].getBufferLength()).toBeGreaterThanOrEqual(1);
		expect(symbolsAtom.get()).toContain("BTC/USD");
	});
});
