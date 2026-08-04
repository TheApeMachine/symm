import { bench, describe } from "vitest";
import {
	graphFramePlan,
	graphStructureKey,
	type MarketGraphFrame,
} from "./graph-surface-store";

const nodeCount = 512;
const graphFrame: MarketGraphFrame = {
	at: "2026-08-04T10:00:00Z",
	nodes: Object.fromEntries(
		Array.from({ length: nodeCount }, (_, index) => [
			`node-${index}`,
			{ id: `node-${index}`, value: index / nodeCount },
		]),
	),
	edges: Array.from({ length: nodeCount - 1 }, (_, index) => ({
		from: `node-${index}`,
		to: `node-${index + 1}`,
		relation: index % 2 === 0 ? "supports" : "conditions",
	})),
};
const displayedKey = graphStructureKey(graphFrame);

describe("graphFramePlan", () => {
	bench("classifies a live 512-node graph frame", () => {
		graphFramePlan(displayedKey, graphFrame);
	});
});
