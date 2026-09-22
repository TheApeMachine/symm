/* Renderer-owned data contract; producers supply evidence and relationships. */
export type GraphNodeKind = "measurement" | "category" | "concept";
export interface GraphNode {
	key: string;
	kind?: GraphNodeKind;
	category?: string;
	measurement: Record<string, unknown>;
}
export interface GraphEdge {
	from: string;
	to: string;
	type: string;
	at: string;
	observedFrom: string;
}
export interface Graph {
	symbol: string;
	at: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
}
