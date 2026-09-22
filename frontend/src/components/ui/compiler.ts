import type { CompiledUINode, CompiledUIRoute } from "./renderer";
import metadataJson from "./ui-component-metadata.generated.json";
import {
	type UIComponentName,
	uiComponents,
} from "./ui-component-registry.generated";

export interface UIDiagnostic {
	nodeId?: string;
	nodeType?: string;
	propName?: string;
	kind:
		| "unknown_component"
		| "invalid_prop"
		| "invalid_prop_type"
		| "invalid_variant"
		| "unexpected_children"
		| "cycle"
		| "missing_route";
	message: string;
}

export interface UICompilationResult {
	routes: CompiledUIRoute[];
	diagnostics: UIDiagnostic[];
}

export interface FlumeConnectionTarget {
	nodeId: string;
	portName: string;
}

export interface FlumeNodeConnections {
	inputs?: Record<string, FlumeConnectionTarget[]>;
	outputs?: Record<string, FlumeConnectionTarget[]>;
}

export interface FlumeGraphNode {
	id?: string;
	type: string;
	connections?: FlumeNodeConnections;
	inputData?: Record<string, any>;
}

export interface FlumeGraph {
	id?: string;
	name?: string;
	nodes?: Record<string, FlumeGraphNode>;
}

interface ComponentMeta {
	name: string;
	exportName: string;
	subPath?: string;
	hasChildren: boolean;
	props: Array<{
		name: string;
		type: "string" | "number" | "boolean" | "select" | "slot";
		options?: string[];
		optional: boolean;
		defaultValue?: any;
	}>;
}

const componentMetadata = metadataJson as Record<string, ComponentMeta>;

/*
parseComponentsPortIndex extracts the numeric order of a components port:
"components" -> 0
"components_1" -> 1
"components_2" -> 2
*/
function parseComponentsPortIndex(portName: string): number {
	if (portName === "components") return 0;
	if (portName.startsWith("components_")) {
		const num = parseInt(portName.slice("components_".length), 10);
		if (!Number.isNaN(num)) return num;
	}
	return Number.MAX_SAFE_INTEGER;
}

/*
compileUI lowers an authored Flume graph into validated CompiledUIRoute structures.
It:
1. Discovers route roots (ui.UIRoute) or top-level UI components.
2. Resolves component types through the generated registry.
3. Validates props against reflected TypeScript metadata (rejecting invalid select options/types).
4. Distinguishes static authored literals from live cross-domain bindings.
5. Recursively traverses structural children and slots with cycle detection.
*/
export function compileUI(
	graph: FlumeGraph | null | undefined,
	options: { strict?: boolean } = {},
): UICompilationResult {
	const diagnostics: UIDiagnostic[] = [];
	const routes: CompiledUIRoute[] = [];

	if (!graph?.nodes || Object.keys(graph.nodes).length === 0) {
		return { routes, diagnostics };
	}

	const nodes = graph.nodes;

	// Helper to compile a single UI component node
	function compileComponentNode(
		nodeId: string,
		ancestors: Set<string>,
	): CompiledUINode | null {
		const node = nodes[nodeId];
		if (!node) {
			diagnostics.push({
				nodeId,
				kind: "unknown_component",
				message: `Node "${nodeId}" referenced in connection does not exist in graph.`,
			});
			return null;
		}

		if (ancestors.has(nodeId)) {
			diagnostics.push({
				nodeId,
				nodeType: node.type,
				kind: "cycle",
				message: `Unsupported recursive cycle detected at node "${nodeId}" (${node.type}).`,
			});
			return null;
		}

		const rawType = node.type;
		const compName = rawType.startsWith("ui.") ? rawType.slice(3) : rawType;

		// 1. Verify component exists in generated registry
		if (!(compName in uiComponents)) {
			diagnostics.push({
				nodeId,
				nodeType: rawType,
				kind: "unknown_component",
				message: `UI component "${compName}" is not registered in uiComponents.`,
			});
			return null;
		}

		const meta = componentMetadata[compName];
		const nextAncestors = new Set(ancestors).add(nodeId);
		const props: Record<string, any> = {};
		let className: string | undefined;

		// 2. Validate and extract authored props from inputData
		const allowedPropNames = new Set(meta?.props?.map((p) => p.name) ?? []);
		allowedPropNames.add("className");
		if (meta?.hasChildren) {
			allowedPropNames.add("children");
		}

		const inputData = node.inputData ?? {};
		for (const [propName, rawEntry] of Object.entries(inputData)) {
			if (propName === "components" || propName.startsWith("components_")) {
				continue;
			}

			const val =
				rawEntry !== null && typeof rawEntry === "object" && "value" in rawEntry
					? rawEntry.value
					: rawEntry;

			if (propName === "className") {
				if (typeof val === "string" && val.trim().length > 0) {
					className = val.trim();
				}
				continue;
			}

			if (meta && !allowedPropNames.has(propName)) {
				diagnostics.push({
					nodeId,
					nodeType: rawType,
					propName,
					kind: "invalid_prop",
					message: `Unknown prop "${propName}" authored on component "${compName}".`,
				});
				continue;
			}

			const propMeta = meta?.props?.find((p) => p.name === propName);
			if (propMeta && val !== undefined && val !== null && val !== "") {
				// Validate select / variant options
				if (
					propMeta.type === "select" &&
					propMeta.options &&
					propMeta.options.length > 0
				) {
					if (!propMeta.options.includes(String(val))) {
						diagnostics.push({
							nodeId,
							nodeType: rawType,
							propName,
							kind: "invalid_variant",
							message: `Invalid value "${val}" for variant/select prop "${propName}" on component "${compName}". Allowed options: ${propMeta.options.join(", ")}.`,
						});
						continue;
					}
				}

				// Validate number
				if (propMeta.type === "number") {
					const num = typeof val === "number" ? val : Number(val);
					if (Number.isNaN(num)) {
						diagnostics.push({
							nodeId,
							nodeType: rawType,
							propName,
							kind: "invalid_prop_type",
							message: `Expected number for prop "${propName}" on component "${compName}", got "${val}".`,
						});
						continue;
					}
					props[propName] = num;
					continue;
				}

				// Validate boolean
				if (propMeta.type === "boolean" && typeof val !== "boolean") {
					diagnostics.push({
						nodeId,
						nodeType: rawType,
						propName,
						kind: "invalid_prop_type",
						message: `Expected boolean for prop "${propName}" on component "${compName}", got "${val}".`,
					});
					continue;
				}
			}

			if (val !== undefined) {
				props[propName] = val;
			}
		}

		// 3. Extract live data bindings and slot connections from inputs
		const inputConnections = node.connections?.inputs ?? {};
		for (const [portName, targets] of Object.entries(inputConnections)) {
			if (portName.startsWith("components")) {
				continue;
			}

			if (!targets || targets.length === 0) {
				continue;
			}

			const firstTarget = targets[0];
			const sourceNode = nodes[firstTarget.nodeId];
			const propMeta = meta?.props?.find((p) => p.name === portName);

			// If port is a structural slot, compile child node
			if (propMeta?.type === "slot" && sourceNode?.type.startsWith("ui.")) {
				const slotChild = compileComponentNode(
					firstTarget.nodeId,
					nextAncestors,
				);
				if (slotChild) {
					props[portName] = slotChild;
				}
				continue;
			}

			// Otherwise, it's a cross-domain live data binding
			props[portName] = {
				binding: {
					node: firstTarget.nodeId,
					port: firstTarget.portName,
				},
			};
		}

		// 4. Resolve structural children from components ports
		const componentPorts = Object.keys(inputConnections)
			.filter((key) => key.startsWith("components"))
			.sort(
				(a, b) => parseComponentsPortIndex(a) - parseComponentsPortIndex(b),
			);

		const children: CompiledUINode[] = [];

		if (componentPorts.length > 0) {
			if (meta && !meta.hasChildren) {
				diagnostics.push({
					nodeId,
					nodeType: rawType,
					kind: "unexpected_children",
					message: `Component "${compName}" has child connections but does not accept children.`,
				});
			} else {
				for (const port of componentPorts) {
					const targets = inputConnections[port] ?? [];
					for (const target of targets) {
						const childNode = compileComponentNode(
							target.nodeId,
							nextAncestors,
						);
						if (childNode) {
							children.push(childNode);
						}
					}
				}
			}
		}

		const compiled: CompiledUINode = {
			name: compName as UIComponentName,
		};

		if (className) compiled.className = className;
		if (Object.keys(props).length > 0) compiled.props = props;
		if (children.length > 0) compiled.children = children;

		return compiled;
	}

	// 5. Discover routes: look for ui.UIRoute nodes
	const routeNodeEntries = Object.entries(nodes).filter(
		([, n]) => n.type === "ui.UIRoute",
	);

	if (routeNodeEntries.length > 0) {
		for (const [routeId, routeNode] of routeNodeEntries) {
			const routeInputData = routeNode.inputData ?? {};
			const pathVal = routeInputData.path?.value ?? routeInputData.path ?? "/";
			const titleVal = routeInputData.title?.value ?? routeInputData.title;

			const inputConnections = routeNode.connections?.inputs ?? {};
			const componentPorts = Object.keys(inputConnections)
				.filter((key) => key.startsWith("components"))
				.sort(
					(a, b) => parseComponentsPortIndex(a) - parseComponentsPortIndex(b),
				);

			const components: CompiledUINode[] = [];
			for (const port of componentPorts) {
				const targets = inputConnections[port] ?? [];
				for (const target of targets) {
					const child = compileComponentNode(target.nodeId, new Set([routeId]));
					if (child) {
						components.push(child);
					}
				}
			}

			routes.push({
				path: String(pathVal),
				title: titleVal ? String(titleVal) : undefined,
				components,
			});
		}
	} else {
		// Fallback: discover top-level UI components (UI nodes without outgoing connections to another UI node)
		const uiNodeIds = Object.entries(nodes)
			.filter(([, n]) => n.type.startsWith("ui."))
			.map(([id]) => id);

		const childUiNodeIds = new Set<string>();
		for (const id of uiNodeIds) {
			const n = nodes[id];
			const inConns = n.connections?.inputs ?? {};
			for (const [port, targets] of Object.entries(inConns)) {
				if (port.startsWith("components")) {
					for (const t of targets) {
						childUiNodeIds.add(t.nodeId);
					}
				}
			}
		}

		const rootUiNodeIds = uiNodeIds.filter((id) => !childUiNodeIds.has(id));
		const components: CompiledUINode[] = [];

		for (const rootId of rootUiNodeIds) {
			const compiled = compileComponentNode(rootId, new Set());
			if (compiled) {
				components.push(compiled);
			}
		}

		if (components.length > 0) {
			routes.push({
				path: "/",
				title: "Autodiscovered UI Root",
				components,
			});
		}
	}

	if (options.strict && diagnostics.length > 0) {
		const errors = diagnostics.map((d) => d.message).join("\n");
		throw new Error(`UI Graph compilation failed:\n${errors}`);
	}

	return { routes, diagnostics };
}
