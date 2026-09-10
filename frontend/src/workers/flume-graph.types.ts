import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import type {
	Coordinate,
	NodeMap,
	TransputType,
} from "#/components/flume/types";

export type ConnectionPathResult = {
	id: string;
	d: string;
};

/*
ConnectionDescriptor names the endpoints of a resolved connection so the
main thread can create or remove SVG shells without recomputing the
roster itself. The worker is the source of truth for which connections
exist; the main thread is the source of truth for the DOM.
*/
export type ConnectionDescriptor = {
	id: string;
	outputNodeId: string;
	outputPortName: string;
	inputNodeId: string;
	inputPortName: string;
};

export type RecalculateResult = {
	paths: ConnectionPathResult[];
	roster: ConnectionDescriptor[];
};

export type GraphPortLayout = {
	nodeId: string;
	portName: string;
	transputType: TransputType;
	offsetX: number;
	offsetY: number;
};

export type GraphNodeLayout = {
	nodeId: string;
	width: number;
	height: number;
};

export type GraphSnapshot = {
	nodes: NodeMap;
	routingMode: EdgeRoutingMode;
	portLayouts: GraphPortLayout[];
	nodeLayouts: GraphNodeLayout[];
};

export type DragUpdateResult = {
	paths: ConnectionPathResult[];
	dragPosition: Coordinate;
};
