import * as flatbuffers from "flatbuffers";
import { describe, expect, it } from "vitest";
import { resonanceStore, symbolsAtom } from "#/collections/app";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { MessageT } from "#/providers/telemetry/telemetry/message";
import { ResonanceT } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceFrameT } from "#/providers/telemetry/telemetry/resonance-frame";
import { dispatchResonanceBuffer, dispatchResonanceRow } from "./rtc";

describe("rtc dispatchResonanceRow", () => {
	it("stores a ringbuffer per symbol in resonanceStore and tracks symbols", () => {
		const mockRes = new ResonanceT();
		mockRes.source = "resonance";
		mockRes.symbol = "BTC/USD";
		mockRes.at = 100n;
		mockRes.confidence = 0.88;

		const mockRow = {
			symbol: () => "BTC/USD",
			unpack: () => mockRes,
		};

		dispatchResonanceRow(mockRow);

		expect(resonanceStore.state["BTC/USD"]).toBeDefined();
		expect(resonanceStore.state["BTC/USD"].getBufferLength()).toBe(1);
		expect(resonanceStore.state["BTC/USD"].toArray()[0]).toEqual(mockRes);
		expect(symbolsAtom.get()).toContain("BTC/USD");
	});

	it("separates ringbuffers by symbol name", () => {
		const ethRes = new ResonanceT();
		ethRes.source = "resonance";
		ethRes.symbol = "ETH/USD";
		ethRes.at = 200n;
		ethRes.confidence = 0.92;

		const mockRow = {
			symbol: () => "ETH/USD",
			unpack: () => ethRes,
		};

		dispatchResonanceRow(mockRow);

		expect(resonanceStore.state["ETH/USD"]).toBeDefined();
		expect(resonanceStore.state["ETH/USD"].getBufferLength()).toBe(1);
		expect(resonanceStore.state["ETH/USD"].toArray()[0]).toEqual(ethRes);
		expect(symbolsAtom.get()).toContain("ETH/USD");
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
		expect(
			resonanceStore.state["AVAX/USD"]
				.toArray()
				.find((r) => r.symbol === "AVAX/USD"),
		).toBeDefined();
		expect(symbolsAtom.get()).toContain("AVAX/USD");
	});
});
