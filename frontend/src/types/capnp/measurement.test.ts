import { describe, expect, it } from "vitest";
import {
	EntityType,
	ENTITY_TYPE_NAMES,
	readWireMeasurement,
	SourceType,
	SOURCE_TYPE_NAMES,
} from "./measurement";

describe("Cap'n Proto Measurement & Metric", () => {
	it("has matching canonical enum names", () => {
		expect(ENTITY_TYPE_NAMES[EntityType.TICKER]).toBe("ticker");
		expect(ENTITY_TYPE_NAMES[EntityType.LEVEL3]).toBe("level3");
		expect(ENTITY_TYPE_NAMES[EntityType.EXECUTION]).toBe("execution");

		expect(SOURCE_TYPE_NAMES[SourceType.HAWKES]).toBe("hawkes");
		expect(SOURCE_TYPE_NAMES[SourceType.MANIFOLD]).toBe("manifold");
		expect(SOURCE_TYPE_NAMES[SourceType.TRAINING]).toBe("training");
		expect(SOURCE_TYPE_NAMES.length).toBe(19);
	});

	it("decodes authoritative Cap'n Proto WireMeasurement produced by Go backend", () => {
		// Base64 of WireMeasurement serialized by nomagique/data/measurement.go MarshalCapnp()
		const base64Payload =
			"AAAAACcAAAAAAAAABwAEABXNhT3+nJcXZQAAAAAAAAAA8VNlAAAAAAAABwAAAAAAAAAAAAAADEAzMzMzMzPrPwAAAAAAAAAADQAAAFoAAAARAAAAOgAAABEAAAA3AAAAKAAAAAAAAQB0ZXN0LWlkLTEyMwAAAAAAQlRDL1VTRAAEAAAABgAAAAAAAAAAAARArkfhehSu8z/NzMzMzMzcPwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAnAAAACAAAAAAAAgANAAAAOgAAAAwAAAACAAEAGQAAAFoAAAAcAAAAAgABAGFjdGlvbgAAAQAAAAAAAAAAAAAAAAAAAAEAAAAqAAAAd2FpdAAAAABjb25maWRlbmNlAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAqAAAAMC44NQAAAAA=";

		const binaryString = atob(base64Payload);
		const bytes = new Uint8Array(binaryString.length);
		for (let i = 0; i < binaryString.length; i++) {
			bytes[i] = binaryString.charCodeAt(i);
		}

		const measurement = readWireMeasurement(bytes.buffer);

		expect(measurement.id).toBe("test-id-123");
		expect(measurement.symbol).toBe("BTC/USD");
		expect(measurement.source).toBe("hawkes");
		expect(measurement.tick).toBe(101n);
		expect(measurement.timestamp).toBe(1700000000n);
		expect(measurement.at).toBe(1700000000123456789n);
		expect(measurement.snr).toBe(3.5);
		expect(measurement.maturity).toBe(0.85);

		// Metrics
		expect(measurement.metrics).toHaveLength(1);
		expect(measurement.metrics[0].raw).toBeCloseTo(2.5, 4);
		expect(measurement.metrics[0].normalized).toBeCloseTo(1.23, 4);
		expect(measurement.metrics[0].standardized).toBeCloseTo(0.45, 4);

		// Metadata
		expect(measurement.metadata["action"]).toBe("wait");
		expect(measurement.metadata["confidence"]).toBe("0.85");

		// Provenance
		expect(measurement.provenance).toContainEqual({
			name: "action",
			value: "wait",
		});
		expect(measurement.provenance).toContainEqual({
			name: "confidence",
			value: "0.85",
		});
	});
});
