import * as Comlink from "comlink";
import type { EdgeRoutingMode } from "#/components/flume/connectionCalculator";
import type { NodeMap, TransputType } from "#/components/flume/types";
import { FlumeGraphEngine } from "#/workers/flume-graph.engine";
import type {
	ConnectionPathResult,
	RecalculateResult,
} from "#/workers/flume-graph.types";

const engine = new FlumeGraphEngine();

const api = {
	setGraph(nodes: NodeMap): void {
		engine.setGraph(nodes);
	},

	setRoutingMode(routingMode: EdgeRoutingMode): void {
		engine.setRoutingMode(routingMode);
	},

	setNodeLayout(nodeId: string, width: number, height: number): void {
		engine.setNodeLayout(nodeId, width, height);
	},

	setPortLayout(
		nodeId: string,
		portName: string,
		transputType: TransputType,
		offsetX: number,
		offsetY: number,
	): void {
		engine.setPortLayout(nodeId, portName, transputType, offsetX, offsetY);
	},

	beginDrag(nodeId: string): void {
		engine.beginDrag(nodeId);
	},

	updateDrag(nodeId: string, x: number, y: number): ConnectionPathResult[] {
		return engine.updateDrag(nodeId, x, y).paths;
	},

	endDrag(nodeId: string, x: number, y: number): ConnectionPathResult[] {
		return engine.endDrag(nodeId, x, y);
	},

	recalculate(): RecalculateResult {
		return engine.recalculate();
	},
};

export type FlumeGraphWorkerApi = typeof api;

Comlink.expose(api);
