import type { Schema, Setting } from "#/service/compute";
import { Colors, Controls, FlumeConfig, getPortBuilders } from "./typeBuilders";
import type { Control } from "./types";

const PORT_PALETTE: Record<string, (typeof Colors)[keyof typeof Colors]> = {
	any: Colors.grey,
	bool: Colors.green,
	number: Colors.blue,
	string: Colors.orange,
	tensor: Colors.purple,
	trigger: Colors.red,
};

const ALL_PORT_TYPES = Object.keys(PORT_PALETTE);

export const normalizePortType = (raw: string): keyof typeof PORT_PALETTE => {
	if (!raw) return "any";
	switch (raw.toLowerCase()) {
		case "tensor":
		case "string":
		case "trigger":
		case "any":
			return raw.toLowerCase() as keyof typeof PORT_PALETTE;
		case "bool":
		case "boolean":
			return "bool";
		case "number":
		case "float":
		case "float64":
		case "float32":
		case "int":
		case "int64":
		case "int32":
		case "uint":
		case "uint64":
		case "scalar":
			return "number";
		case "primitive":
			return "tensor";
		default:
			return "any";
	}
};

const settingToControl = (param: Setting): Control => {
	const label = param.name;
	const name = param.name;

	if (param.type === "bool" || param.type === "boolean") {
		return Controls.checkbox({ label, name, defaultValue: false });
	}

	if (
		param.type === "number" ||
		param.type === "int" ||
		param.type === "float" ||
		param.type === "scalar"
	) {
		return Controls.number({ label, name, defaultValue: 0 });
	}

	return Controls.text({ label, name, defaultValue: "" });
};

const registerPortTypes = (config: FlumeConfig) => {
	for (const [portType, color] of Object.entries(PORT_PALETTE)) {
		config.addPortType({
			type: portType,
			name: portType,
			label: portType,
			color,
			acceptTypes: ALL_PORT_TYPES,
		});
	}
};

const registerBuiltinNodeTypes = (config: FlumeConfig) => {
	// Source nodes
	const registerSource = (type: string, label: string) => {
		config.addNodeType({
			type,
			label,
			category: "Orchestration",
			description: "Ingress data stream source",
			initialWidth: 280,
			inputs: [],
			outputs: (ports) => [
				ports.any({ name: "out", label: "Out" }),
				ports.any({ name: "value", label: "Value" }),
			],
		});
	};
	registerSource("data.Source", "Source");
	registerSource("source", "Source");

	// Sink nodes
	const registerSink = (type: string, label: string) => {
		config.addNodeType({
			type,
			label,
			category: "Orchestration",
			description: "Pipeline terminal data sink",
			initialWidth: 280,
			inputs: (ports) => [
				ports.any({ name: "in", label: "In" }),
				ports.any({ name: "value", label: "Value" }),
			],
			outputs: [],
		});
	};
	registerSink("data.Sink", "Sink");
	registerSink("sink", "Sink");

	// Master pipeline orchestration stages
	const stages: Array<{ type: string; label: string; desc: string }> = [
		{
			type: "pipeline.Signals",
			label: "Signals",
			desc: "Streaming signal extraction grid",
		},
		{
			type: "signals",
			label: "Signals",
			desc: "Streaming signal extraction grid",
		},
		{
			type: "pipeline.Logic",
			label: "Logic",
			desc: "Associative cognition & attractor basin logic",
		},
		{
			type: "logic",
			label: "Logic",
			desc: "Associative cognition & attractor basin logic",
		},
		{
			type: "ui.Broadcast",
			label: "UI Broadcast",
			desc: "Telemetry binary frame broadcast to UI hub",
		},
		{
			type: "ui",
			label: "UI Broadcast",
			desc: "Telemetry binary frame broadcast to UI hub",
		},
		{
			type: "pipeline.Execution",
			label: "Execution",
			desc: "Execution policy and paper/live order submission",
		},
		{
			type: "execution",
			label: "Execution",
			desc: "Execution policy and paper/live order submission",
		},
		{
			type: "data.Extract",
			label: "Extract",
			desc: "Extracts a scalar path from structured data",
		},
		{
			type: "data.Select",
			label: "Select",
			desc: "Selects nested data properties",
		},
		{
			type: "associative.Grid",
			label: "Associative Grid",
			desc: "Associative memory grid",
		},
		{
			type: "temporal.Transition",
			label: "Transition",
			desc: "Temporal transition operator",
		},
		{
			type: "cognition.Associate",
			label: "Associate",
			desc: "Associative feature mapping",
		},
		{
			type: "cognition.Reinforce",
			label: "Reinforce",
			desc: "Cognitive reward reinforcement",
		},
		{
			type: "cognition.Attractor",
			label: "Attractor",
			desc: "Attractor basin dynamics",
		},
		{
			type: "cognition.Classification",
			label: "Classification",
			desc: "Attractor state classification",
		},
		{
			type: "execution.Decide",
			label: "Decide",
			desc: "Execution decision logic",
		},
		{
			type: "execution.Gate",
			label: "Gate",
			desc: "Risk and threshold gating",
		},
		{
			type: "execution.Submit",
			label: "Submit",
			desc: "Order execution submission",
		},
		{
			type: "transport.Collect",
			label: "Collect",
			desc: "Collects items into batches",
		},
		{
			type: "transport.Discard",
			label: "Discard",
			desc: "Discards stream data",
		},
		{
			type: "transport.Pace",
			label: "Pace",
			desc: "Rate limits stream throughput",
		},
		{
			type: "transport.Process",
			label: "Process",
			desc: "External subprocess execution",
		},
		{
			type: "temporal.Delay",
			label: "Delay",
			desc: "Temporal lookback delay buffer",
		},
	];

	for (const s of stages) {
		if (!config.nodeTypes[s.type]) {
			config.addNodeType({
				type: s.type,
				label: s.label,
				category: "Architecture",
				description: s.desc,
				initialWidth: 280,
				inputs: (ports) => [
					ports.any({ name: "in", label: "In" }),
					ports.any({ name: "value", label: "Value" }),
				],
				outputs: (ports) => [
					ports.any({ name: "out", label: "Out" }),
					ports.any({ name: "value", label: "Value" }),
				],
			});
		}
	}

	// Legacy gate and scalar
	if (!config.nodeTypes.gate) {
		config.addNodeType({
			type: "gate",
			label: "Gate",
			category: "Built-in",
			description: "Conditional tensor pass-through",
			initialWidth: 300,
			inputs: (ports) => [
				ports.tensor({ name: "in", label: "In" }),
				ports.bool({
					name: "open",
					label: "Open",
					controls: [
						Controls.checkbox({
							name: "open",
							label: "Open",
							defaultValue: true,
						}),
					],
				}),
			],
			outputs: (ports) => [ports.tensor({ name: "out", label: "Out" })],
		});
	}

	if (!config.nodeTypes.scalar) {
		config.addNodeType({
			type: "scalar",
			label: "Scalar",
			category: "Built-in",
			description: "Number math",
			initialWidth: 300,
			inputs: (ports) => [
				ports.number({
					name: "x",
					label: "X",
					controls: [
						Controls.number({ name: "x", label: "X", defaultValue: 0 }),
					],
				}),
				ports.number({
					name: "y",
					label: "Y",
					controls: [
						Controls.number({ name: "y", label: "Y", defaultValue: 0 }),
					],
				}),
			],
			outputs: (ports) => [
				ports.number({ name: "sum", label: "Sum" }),
				ports.number({ name: "diff", label: "Diff" }),
			],
		});
	}
};

const schemaToNodeType = (config: FlumeConfig, schema: Schema) => {
	if (!schema?.op || typeof schema.op !== "string") {
		return;
	}

	if (config.nodeTypes[schema.op]) {
		return;
	}

	if (!Array.isArray(schema.inputs) || !Array.isArray(schema.outputs)) {
		return;
	}

	const ports = getPortBuilders(config.portTypes);

	const inputPorts = schema.inputs.map((port) => {
		const portType = normalizePortType(port.type);

		return ports[portType]({
			name: port.name,
			label: port.name,
		});
	});

	const configParams = Array.isArray(schema.config) ? schema.config : [];

	if (configParams.length > 0) {
		inputPorts.unshift(
			ports.any({
				name: "_config",
				label: "Config",
				hidePort: true,
				controls: configParams.map(settingToControl),
			}),
		);
	}

	config.addNodeType({
		type: schema.op,
		label: schema.label || schema.name || schema.op,
		description: schema.description,
		category: schema.category || "Operations",
		initialWidth: 300,
		inputs: inputPorts,
		outputs: schema.outputs.map((port) => {
			const portType = normalizePortType(port.type);

			return ports[portType]({
				name: port.name,
				label: port.name,
			});
		}),
	});
};

/*
Ensures any unknown node type found in an imported graph is dynamically registered
so it is never dropped during reconciliation.
*/
export const ensureNodeType = (
	config: FlumeConfig,
	typeName: string,
	category = "Custom",
) => {
	if (!typeName || config.nodeTypes[typeName]) {
		return;
	}

	const ports = getPortBuilders(config.portTypes);
	config.addNodeType({
		type: typeName,
		label: typeName,
		category,
		initialWidth: 280,
		inputs: [
			ports.any({ name: "in", label: "In" }),
			ports.any({ name: "value", label: "Value" }),
		],
		outputs: [
			ports.any({ name: "out", label: "Out" }),
			ports.any({ name: "value", label: "Value" }),
		],
	});
};

/*
buildFlumeConfigFromSchemas registers port types, built-in architecture nodes,
and one Flume node type per backend operation schema.
*/
export const buildFlumeConfigFromSchemas = (
	schemas: Record<string, Schema>,
): FlumeConfig => {
	const config = new FlumeConfig();

	registerPortTypes(config);
	registerBuiltinNodeTypes(config);

	for (const schema of Object.values(schemas)) {
		schemaToNodeType(config, schema);
	}

	return config;
};
