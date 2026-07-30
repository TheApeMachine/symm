/**
 * Graph data structure for neural network visualization
 *
 * Manages nodes and edges in a graph structure optimized for GPU-based
 * force-directed layout simulation. Supports temporal data for animating
 * network activity over time.
 */

export interface GraphSettings {
	epoch: string;
	epochFormat: string;
	source: string;
	target: string;
}

export interface NodeData {
	[key: string]: unknown;
}

export interface Node {
	id: number;
	edges: number[];
	data: NodeData[];
}

export interface Edge {
	source: string;
	target: string;
	id: number;
	data: NodeData[];
}

export interface GraphData {
	nodes: Record<string, Node>;
	edges: Record<string, Edge>;
	settings: GraphSettings;
}

const DEFAULT_SETTINGS: GraphSettings = {
	epoch: "Event Time",
	epochFormat: "YYYY-M-D H:m:s",
	source: "source",
	target: "target",
};

/**
 * Graph class for managing network topology
 *
 * Provides methods for adding nodes and edges, and generating texture data
 * for GPU-based rendering. The graph supports temporal data, allowing
 * visualization of network activity over time.
 */
export class Graph {
	public nodes: Record<string, Node> = {};
	public edges: Record<string, Edge> = {};
	public settings: GraphSettings;

	private nodesCount = 0;
	private edgesCount = 0;

	constructor(settings: Partial<GraphSettings> = {}) {
		this.settings = { ...DEFAULT_SETTINGS, ...settings };
	}

	private getNode(nodeName: string): Node | undefined {
		return this.nodes[nodeName];
	}

	private getEdge(source: string, target: string): Edge | undefined {
		return this.edges[`${source}<>${target}`];
	}

	private createEdge(source: string, target: string, data?: NodeData): void {
		let edge = this.getEdge(source, target) ?? this.getEdge(target, source);

		if (!edge) {
			edge = {
				source,
				target,
				id: this.edgesCount,
				data: [],
			};
			this.edgesCount++;
			this.edges[`${source}<>${target}`] = edge;
		}

		if (data) {
			edge.data.push(data);
		}
	}

	/**
	 * Add a node to the graph
	 *
	 * Creates a new node if it doesn't exist, or updates existing node's data.
	 * Returns the node object for chaining.
	 */
	addNode(nodeName: string, nodeData?: NodeData): Node | null {
		if (!nodeName) {
			return null;
		}

		let node = this.getNode(nodeName);

		if (!node) {
			node = {
				id: this.nodesCount,
				edges: [],
				data: [],
			};
			this.nodesCount++;
			this.nodes[nodeName] = node;
		}

		if (nodeData) {
			node.data.push(nodeData);
		}

		return node;
	}

	/**
	 * Add an edge between two nodes
	 *
	 * Creates both nodes if they don't exist, and creates the edge.
	 * Self-loops (source === target) only create a node with data.
	 */
	addEdge(source: string, target: string, data?: NodeData): void {
		if (source === target) {
			this.addNode(source, data);
			return;
		}

		const fromNode = this.addNode(source, data);
		const toNode = this.addNode(target, data);

		if (fromNode && toNode) {
			fromNode.edges.push(toNode.id);
			toNode.edges.push(fromNode.id);
			this.createEdge(source, target, data);
		}
	}

	/**
	 * Add a row from CSV data
	 *
	 * Uses the configured source and target column names to create an edge.
	 */
	addCSVRow(data: NodeData): void {
		const source = data[this.settings.source] as string;
		const target = data[this.settings.target] as string;
		this.addEdge(source, target, data);
	}

	/**
	 * Get array of unique edge IDs for each node
	 *
	 * Used for building texture data for GPU simulation.
	 */
	getNodesAndEdgesArray(): number[][] {
		const edgesArray: number[][] = [];

		Object.values(this.nodes)
			.filter(Boolean)
			.forEach((node) => {
				edgesArray[node.id] = [...new Set(node.edges ?? [])];
			});

		return edgesArray;
	}

	/**
	 * Get lookup table mapping node names to texture positions
	 *
	 * Used for correlating node data with GPU texture coordinates.
	 */
	getLookupTable(
		nodesWidth: number,
	): Record<string, { texPos: [number, number]; color?: number[] }> {
		const lookupTable: Record<
			string,
			{ texPos: [number, number]; color?: number[] }
		> = {};
		let i = 0;

		Object.keys(this.nodes).forEach((key) => {
			const texStartX = (i % nodesWidth) / nodesWidth;
			const texStartY = Math.floor(i / nodesWidth) / nodesWidth;

			lookupTable[key] = { texPos: [texStartX, texStartY] };
			i++;
		});

		return lookupTable;
	}

	/**
	 * Get epoch (timestamp) data for nodes or edges
	 *
	 * Extracts timestamps from node/edge data for temporal visualization.
	 */
	getEpochTextureArray(type: "nodes" | "edges"): number[][] {
		const thing = type === "nodes" ? this.nodes : this.edges;
		const epochArray: number[][] = [];

		Object.values(thing)
			.filter(Boolean)
			.forEach((value) => {
				const epochs: number[] = [];

				(value.data ?? [])
					.filter(Boolean)
					.forEach((dvalue: Record<string, unknown>) => {
						const epochValue = dvalue[this.settings.epoch];
						if (epochValue) {
							// Parse timestamp - simplified from moment.js
							const timestamp = new Date(epochValue as string).getTime() / 1000;
							if (!Number.isNaN(timestamp)) {
								epochs.push(timestamp);
							}
						}
					});

				epochArray[value.id] = [...new Set(epochs)];
			});

		return Array.from(
			{ length: epochArray.length },
			(_, i) => epochArray[i] ?? [],
		);
	}

	/**
	 * Get the total number of nodes
	 */
	getNodeCount(): number {
		return this.nodesCount;
	}

	/**
	 * Get the total number of edges
	 */
	getEdgeCount(): number {
		return this.edgesCount;
	}

	/**
	 * Clear all nodes and edges
	 */
	clear(): void {
		this.nodes = {};
		this.edges = {};
		this.nodesCount = 0;
		this.edgesCount = 0;
	}

	/**
	 * Load graph from serialized data
	 */
	loadFromData(data: GraphData): void {
		this.nodes = data.nodes ?? {};
		this.edges = data.edges ?? {};
		this.settings = { ...DEFAULT_SETTINGS, ...data.settings };

		// Normalize null data arrays that Go serializes from nil slices
		for (const node of Object.values(this.nodes)) {
			if (node) node.data = (node.data ?? []).filter(Boolean);
		}
		for (const edge of Object.values(this.edges)) {
			if (edge) edge.data = (edge.data ?? []).filter(Boolean);
		}

		this.nodesCount = Object.keys(this.nodes).length;
		this.edgesCount = Object.keys(this.edges).length;
	}

	/**
	 * Export graph to serializable format
	 */
	toJSON(): GraphData {
		return {
			nodes: this.nodes,
			edges: this.edges,
			settings: this.settings,
		};
	}
}

/**
 * Graph generators for testing and demos
 */
export const generators = {
	/**
	 * Generate a star graph with hub nodes connected to leaf nodes
	 */
	star(graph: Graph, hubCount = 50, leafCount = 50): void {
		for (let i = 0; i < hubCount; i++) {
			for (let j = hubCount; j < hubCount + leafCount; j++) {
				graph.addEdge(String(i), String(j));
			}
		}
	},

	/**
	 * Generate a balanced binary tree
	 */
	balancedTree(graph: Graph, depth = 9): void {
		const count = 2 ** depth;

		if (depth === 0) {
			graph.addNode("1");
			return;
		}

		for (let level = 1; level < count; level++) {
			const root = level;
			const left = root * 2;
			const right = root * 2 + 1;

			graph.addEdge(String(root), String(left));
			graph.addEdge(String(root), String(right));
		}
	},

	/**
	 * Generate a 3D cube/grid graph
	 */
	cube(graph: Graph, size = 5): void {
		const n = size;
		const m = size;
		const z = size;

		for (let k = 0; k < z; k++) {
			for (let i = 0; i < n; i++) {
				for (let j = 0; j < m; j++) {
					const level = k * n * m;
					const node = i + j * n + level;

					if (i > 0) graph.addEdge(String(node), String(i - 1 + j * n + level));
					if (j > 0)
						graph.addEdge(String(node), String(i + (j - 1) * n + level));
					if (k > 0)
						graph.addEdge(String(node), String(i + j * n + (k - 1) * n * m));
				}
			}
		}
	},

	/**
	 * Generate a chain/path graph
	 */
	chain(graph: Graph, length = 100): void {
		graph.addNode("0");
		for (let i = 1; i <= length; i++) {
			graph.addEdge(String(i - 1), String(i));
		}
	},
};
