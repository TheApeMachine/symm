import type { ConfigParam, Schema } from "#/service/compute";
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

const normalizePortType = (raw: string): keyof typeof PORT_PALETTE => {
	switch (raw) {
		case "tensor":
		case "string":
		case "bool":
		case "number":
		case "trigger":
		case "any":
			return raw;
		case "float":
		case "int":
		case "scalar":
			return "number";
		default:
			return "any";
	}
};

const configParamToControl = (param: ConfigParam): Control => {
	const label = param.name;
	const name = param.name;

	if (param.type === "bool" || param.type === "boolean") {
		return Controls.checkbox({
			label,
			name,
			defaultValue: Boolean(param.default ?? false),
		});
	}

	if (
		param.type === "number" ||
		param.type === "int" ||
		param.type === "float" ||
		param.type === "scalar"
	) {
		return Controls.number({
			label,
			name,
			defaultValue: Number(param.default ?? 0),
		});
	}

	return Controls.text({
		label,
		name,
		defaultValue: String(param.default ?? ""),
	});
};

const registerPortTypes = (config: FlumeConfig) => {
	for (const [portType, color] of Object.entries(PORT_PALETTE)) {
		config.addPortType({
			type: portType,
			name: portType,
			label: portType,
			color,
		});
	}
};

const registerBuiltinNodeTypes = (config: FlumeConfig) => {
	config
		.addNodeType({
			type: "source",
			label: "Source",
			category: "Built-in",
			description: "Tensor source",
			initialWidth: 280,
			inputs: [],
			outputs: (ports) => [ports.tensor({ name: "value", label: "Value" })],
		})
		.addNodeType({
			type: "sink",
			label: "Sink",
			category: "Built-in",
			description: "Tensor sink",
			initialWidth: 280,
			inputs: (ports) => [ports.tensor({ name: "value", label: "Value" })],
			outputs: [],
		})
		.addNodeType({
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
		})
		.addNodeType({
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
				controls: configParams.map(configParamToControl),
			}),
		);
	}

	config.addNodeType({
		type: schema.op,
		label: schema.label || schema.name || schema.op,
		description: schema.description,
		category: schema.category || "Operations",
		initialWidth: schema.initial_width || 300,
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
buildFlumeConfigFromSchemas registers port types, built-in nodes, and one
Flume node type per backend operation schema.
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
