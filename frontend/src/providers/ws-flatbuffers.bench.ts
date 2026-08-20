import { bench, describe } from "vitest";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { decodeTelemetryTable } from "#/providers/ws-flatbuffers";

const frame = {
	unpack: () => ({
		rows: [
			{
				symbol: "BTC/USD",
				layers: Array.from({ length: 3 }, () => ({
					state: Array.from({ length: 44 }, (_, index) => index / 44),
					prediction: Array.from(
						{ length: 44 },
						(_, index) => index / 88,
					),
				})),
			},
		],
	}),
};

describe("decodeTelemetryTable", () => {
	bench("decodes predictive state and overlay vectors", () => {
		decodeTelemetryTable(Frame.ResonanceFrame, frame);
	});
});
