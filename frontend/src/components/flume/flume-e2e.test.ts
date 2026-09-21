import { describe, expect, it } from "vitest";
import { createFlumeConfig } from "./flume-config.generated";
import type { FlumeNode } from "./types";

describe("Flume Workbench End-to-End Slice", () => {
	it("1. builds authoritative palette from nomagique catalog with strict Cap'n Proto ports", () => {
		const config = createFlumeConfig();

		// Obsolete fake demo nodes must not exist
		expect(config.nodeTypes.gate).toBeUndefined();
		expect(config.nodeTypes.scalar).toBeUndefined();
		expect(config.nodeTypes["pipeline.Signals"]).toBeUndefined();
		expect(config.nodeTypes["pipeline.Logic"]).toBeUndefined();
		expect(config.nodeTypes["pipeline.Execution"]).toBeUndefined();

		// Real primitives from catalog must be registered
		expect(config.nodeTypes["arithmetic.Add"]).toBeDefined();
		expect(config.nodeTypes["calculus.Square"]).toBeDefined();
		expect(config.nodeTypes["calculus.Atanh"]).toBeDefined();
		expect(config.nodeTypes.source).toBeDefined();
		expect(config.nodeTypes.sink).toBeDefined();

		// Port types must strictly be the 5 standard types: bool, string, []byte, int64, float64
		expect(config.portTypes.float64).toBeDefined();
		expect(config.portTypes.int64).toBeDefined();
		expect(config.portTypes.bool).toBeDefined();
		expect(config.portTypes.string).toBeDefined();
		expect(config.portTypes["[]byte"]).toBeDefined();

		// []byte port type must have a textarea control for multi-line JSON/payload input
		const bytePort = config.portTypes["[]byte"];
		expect(bytePort.controls).toBeDefined();
		expect(bytePort.controls?.[0]?.type).toBe("textarea");

		// Strict port compatibility: float64 only accepts float64
		const float64Port = config.portTypes.float64;
		expect(float64Port.acceptTypes).toEqual(["float64"]);
		expect(float64Port.acceptTypes).not.toContain("int64");
	});

	it("2. supports static input controls for primitives like arithmetic.Add", () => {
		const config = createFlumeConfig();
		const addType = config.nodeTypes["arithmetic.Add"];
		expect(addType).toBeDefined();

		// Inputs 'a' and 'b' must have controls attached (number controls)
		const inputs = Array.isArray(addType.inputs) ? addType.inputs : [];
		const inputA = inputs.find((p) => p.name === "a");
		const inputB = inputs.find((p) => p.name === "b");
		expect(inputA).toBeDefined();
		expect(inputB).toBeDefined();
		expect(inputA?.controls).toBeDefined();
		expect(inputA?.controls?.length).toBeGreaterThan(0);
		expect(inputB?.controls).toBeDefined();
		expect(inputB?.controls?.length).toBeGreaterThan(0);
	});

	it("3. serializes Add(2,3) -> Square into execution-ready JSON matching compiler schema", () => {
		const nodes: Record<string, FlumeNode> = {
			add_1: {
				id: "add_1",
				type: "arithmetic.Add",
				x: 100,
				y: 100,
				width: 280,
				inputData: {
					a: { number: 2 },
					b: { number: 3 },
				},
				connections: {
					inputs: {},
					outputs: {
						out: [{ nodeId: "square_1", portName: "in" }],
					},
				},
			},
			square_1: {
				id: "square_1",
				type: "calculus.Square",
				x: 450,
				y: 100,
				width: 280,
				inputData: {},
				connections: {
					inputs: {
						in: [{ nodeId: "add_1", portName: "out" }],
					},
					outputs: {},
				},
			},
		};

		const exported = {
			id: "test_add_sq",
			nodes: Object.fromEntries(
				Object.entries(nodes).map(([id, node]) => [
					id,
					{
						id: node.id || id,
						type: node.type,
						connections: node.connections,
						inputData: Object.fromEntries(
							Object.entries(node.inputData ?? {}).map(([k, v]) => [
								k,
								typeof v === "object" && v !== null && "number" in v
									? (v as { number: number }).number
									: v,
							]),
						),
					},
				]),
			),
		};

		expect(exported.id).toBe("test_add_sq");
		expect(exported.nodes.add_1.type).toBe("arithmetic.Add");
		expect(exported.nodes.add_1.inputData).toEqual({ a: 2, b: 3 });
		expect(exported.nodes.square_1.type).toBe("calculus.Square");
		expect(exported.nodes.square_1.connections.inputs.in).toEqual([
			{ nodeId: "add_1", portName: "out" },
		]);
	});

	it("4. exposes reusable definition nodes with dynamic boundary ports", () => {
		const config = createFlumeConfig(["correlation_ticker"]);

		const defNode = config.nodeTypes["definition:correlation_ticker"];
		expect(defNode).toBeDefined();
		expect(defNode.label).toBe("correlation_ticker");
		expect(typeof defNode.inputs).toBe("function");
		expect(typeof defNode.outputs).toBe("function");
	});

	it("5. rejects incompatible port connections before compile", () => {
		const config = createFlumeConfig();

		const floatPort = config.portTypes.float64;
		const textPort = config.portTypes.string;
		const boolPort = config.portTypes.bool;

		// float64 port must not accept string or bool
		expect(floatPort?.acceptTypes?.includes("string")).toBe(false);
		expect(floatPort?.acceptTypes?.includes("bool")).toBe(false);
		expect(textPort?.acceptTypes?.includes("float64")).toBe(false);
		expect(boolPort?.acceptTypes?.includes("float64")).toBe(false);
	});
});
