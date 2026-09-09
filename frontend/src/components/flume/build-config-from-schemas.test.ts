import { describe, expect, it } from "vitest";
import { buildFlumeConfigFromSchemas } from "./build-config-from-schemas";

describe("buildFlumeConfigFromSchemas", () => {
	it("registers built-in demo node types", () => {
		const config = buildFlumeConfigFromSchemas({});

		expect(config.nodeTypes.source).toBeDefined();
		expect(config.nodeTypes.gate).toBeDefined();
		expect(config.nodeTypes.sink).toBeDefined();
	});

	it("skips malformed and duplicate operation schemas", () => {
		const config = buildFlumeConfigFromSchemas({
			bad: { op: "bad.op" } as never,
			good: {
				kind: "operation",
				category: "math",
				op: "math.test",
				name: "math.test",
				label: "Test",
				description: "Test op",
				initial_width: 280,
				inputs: [{ name: "x", type: "tensor", description: "" }],
				outputs: [{ name: "y", type: "tensor", description: "" }],
				config: [],
			},
			duplicate: {
				kind: "operation",
				category: "math",
				op: "math.test",
				name: "math.test.duplicate",
				label: "Duplicate",
				description: "Duplicate op",
				initial_width: 280,
				inputs: [{ name: "x", type: "tensor", description: "" }],
				outputs: [{ name: "y", type: "tensor", description: "" }],
				config: [],
			},
		});

		expect(config.nodeTypes["math.test"]).toBeDefined();
		expect(config.nodeTypes.source).toBeDefined();
	});

	it("registers operation schemas when config is null", () => {
		const config = buildFlumeConfigFromSchemas({
			"math.null_config": {
				kind: "operation",
				category: "math",
				op: "math.null_config",
				name: "math.null_config",
				label: "Null config",
				description: "Op with null config from backend JSON",
				initial_width: 280,
				inputs: [{ name: "x", type: "tensor", description: "" }],
				outputs: [{ name: "y", type: "tensor", description: "" }],
				config: null,
			},
		});

		expect(config.nodeTypes["math.null_config"]).toBeDefined();
	});
});
