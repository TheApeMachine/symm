import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { resonanceStore, symbolsAtom } from "#/collections/app";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MeasurementsFrameT } from "#/providers/telemetry/telemetry/measurements-frame";
import { MessageT } from "#/providers/telemetry/telemetry/message";
import { ResonanceT } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceFrameT } from "#/providers/telemetry/telemetry/resonance-frame";
import { dispatchResonanceBuffer, dispatchResonanceRow } from "./rtc";

describe("rtc dispatchResonanceRow", () => {
	it("stores a ringbuffer per symbol in resonanceStore and tracks symbols", () => {
		const mockMeasurement = new MeasurementT(
			"m1",
			"resonance",
			"BTC/USD",
			1n,
			"",
			100n,
			90n,
			10n,
			0n,
			0n,
			1,
			2.5,
			true,
			[],
			[],
		);

		const mockRow = {
			symbol: () => "BTC/USD",
			unpack: () => mockMeasurement,
		};

		dispatchResonanceRow(mockRow);

		expect(resonanceStore.state["BTC/USD"]).toBeDefined();
		expect(resonanceStore.state["BTC/USD"].getBufferLength()).toBe(1);
		expect(resonanceStore.state["BTC/USD"].toArray()[0]).toEqual(mockMeasurement);
		expect(symbolsAtom.get()).toContain("BTC/USD");
	});

	it("separates ringbuffers by symbol name", () => {
		const ethMeasurement = new MeasurementT(
			"m2",
			"resonance",
			"ETH/USD",
			2n,
			"",
			200n,
			190n,
			10n,
			0n,
			0n,
			1,
			3.0,
			true,
			[],
			[],
		);

		const mockRow = {
			symbol: () => "ETH/USD",
			unpack: () => ethMeasurement,
		};

		dispatchResonanceRow(mockRow);

		expect(resonanceStore.state["ETH/USD"]).toBeDefined();
		expect(resonanceStore.state["ETH/USD"].getBufferLength()).toBe(1);
		expect(resonanceStore.state["ETH/USD"].toArray()[0]).toEqual(ethMeasurement);
		expect(symbolsAtom.get()).toContain("ETH/USD");
	});

	it("handles FlatBuffer Message containing MeasurementsFrame", () => {
		const measurement = new MeasurementT(
			"m3",
			"resonance",
			"SOL/USD",
			3n,
			"",
			300n,
			290n,
			10n,
			0n,
			0n,
			1,
			4.0,
			true,
			[],
			[],
		);
		const frame = new MeasurementsFrameT([measurement]);
		const msg = new MessageT(1n, Frame.MeasurementsFrame, frame);

		const builder = new flatbuffers.Builder(1024);
		const offset = msg.pack(builder);
		builder.finish(offset, "SYMM");

		const buffer = new flatbuffers.ByteBuffer(builder.asUint8Array());
		dispatchResonanceBuffer(buffer);

		expect(resonanceStore.state["SOL/USD"]).toBeDefined();
		expect(resonanceStore.state["SOL/USD"].toArray().find((r) => r.symbol === "SOL/USD")).toBeDefined();
		expect(symbolsAtom.get()).toContain("SOL/USD");
	});

	it("handles FlatBuffer Message containing ResonanceFrame", () => {
		const res = new ResonanceT();
		res.source = "resonance";
		res.symbol = "AVAX/USD";
		res.at = 400n;
		res.confidence = 0.95;

		const frame = new ResonanceFrameT([res]);
		const msg = new MessageT(2n, Frame.ResonanceFrame, frame);

		const builder = new flatbuffers.Builder(1024);
		const offset = msg.pack(builder);
		builder.finish(offset, "SYMM");

		const buffer = new flatbuffers.ByteBuffer(builder.asUint8Array());
		dispatchResonanceBuffer(buffer);

		expect(resonanceStore.state["AVAX/USD"]).toBeDefined();
		expect(resonanceStore.state["AVAX/USD"].toArray().find((r) => r.symbol === "AVAX/USD")).toBeDefined();
		expect(symbolsAtom.get()).toContain("AVAX/USD");
	});
});
