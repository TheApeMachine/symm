import { createRef, useCallback, useEffect, useState } from "react";
import { resizeCanvas } from "#/components/terminal/canvas";
import {
	buildScene,
	drawEvidenceGraph,
	type GraphHit,
	type GraphNodePosition,
	type GraphScene,
	graphVisualKey,
	hitTest,
	nodeIdentity,
} from "#/components/terminal/evidence-graph-viz";
import { GraphInspector } from "#/components/terminal/thesis-graph-inspector";
import { graphStore } from "#/providers/ws-stores";
import type { Graph, GraphEdge, GraphNode } from "#/types/thesis";

type HoverState = {
	hit: GraphHit;
	x: number;
	y: number;
};

const TWEEN_MS = 420;

const canvasRef = createRef<HTMLCanvasElement>();
let graph: Graph | null = null;
let targetScene: GraphScene | null = null;
let renderScene: GraphScene | null = null;
let renderedByIdentity = new Map<string, GraphNodePosition>();
let topologyKey = "";
let hoverKey: string | null = null;
let animation: number | null = null;
let tweenStart = 0;
let fromByIdentity = new Map<string, GraphNodePosition>();

const asRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

/*
evidenceGraphFor cuts one symbol's evidence out of the run-wide graph.

The engine publishes a single graph keyed by node id, not one frame per symbol,
so the modal selects the nodes that belong to its symbol and keeps the edges
whose two ends both survived that cut. Selecting rows is all that happens here —
no weight, relation or confidence is recomputed, so the picture cannot claim
anything the engine did not.
*/
export const evidenceGraphFor = (
	frame: unknown,
	symbol: string,
): Graph | null => {
	const record = asRecord(frame);
	const rawNodes = asRecord(record?.nodes);

	if (record === null || rawNodes === null || symbol === "") {
		return null;
	}

	const nodes: GraphNode[] = [];
	const keys = new Set<string>();

	for (const [key, value] of Object.entries(rawNodes)) {
		const node = asRecord(value);

		if (node === null || node.symbol !== symbol) {
			continue;
		}

		keys.add(key);
		nodes.push({
			key,
			kind: node.kind === "category" ? "category" : "measurement",
			category: typeof node.kind === "string" ? node.kind : undefined,
			measurement: node,
		});
	}

	if (nodes.length === 0) {
		return null;
	}

	const edges: GraphEdge[] = [];

	for (const value of Object.values(asRecord(record.edges) ?? {})) {
		const edge = asRecord(value);

		if (
			edge === null ||
			typeof edge.from !== "string" ||
			typeof edge.to !== "string" ||
			!keys.has(edge.from) ||
			!keys.has(edge.to)
		) {
			continue;
		}

		edges.push({
			from: edge.from,
			to: edge.to,
			type: typeof edge.relation === "string" ? edge.relation : "",
			at: typeof edge.at === "string" ? edge.at : "",
			observedFrom: typeof edge.at === "string" ? edge.at : "",
		});
	}

	return {
		symbol,
		at: typeof record.at === "string" ? record.at : "",
		nodes,
		edges,
	};
};

const easeInOut = (t: number): number =>
	t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2;

/*
draw paints the current render scene onto the bound canvas.
*/
const draw = (nextHoverKey: string | null) => {
	const canvas = canvasRef.current;

	if (canvas === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	drawEvidenceGraph(context, width, height, graph, renderScene, nextHoverKey);
};

/*
composeRenderScene builds the render scene at tween progress p (0..1).
*/
const composeRenderScene = (progress: number) => {
	if (targetScene === null) {
		renderScene = null;
		return;
	}

	const positions = new Map<string, GraphNodePosition>();
	const rendered = new Map<string, GraphNodePosition>();
	const eased = easeInOut(progress);

	for (const [key, node] of targetScene.nodes) {
		const identity = nodeIdentity(node);
		const to = targetScene.positions.get(key);

		if (to === undefined) {
			continue;
		}

		const from = fromByIdentity.get(identity) ?? to;
		const position = {
			x: from.x + (to.x - from.x) * eased,
			y: from.y + (to.y - from.y) * eased,
		};

		positions.set(key, position);
		rendered.set(identity, position);
	}

	renderScene = {
		positions,
		nodes: targetScene.nodes,
		width: targetScene.width,
		height: targetScene.height,
	};
	renderedByIdentity = rendered;
};

const animate = (now: number) => {
	const elapsed = now - tweenStart;
	const progress = Math.min(1, elapsed / TWEEN_MS);

	composeRenderScene(progress);
	draw(hoverKey);

	if (progress < 1) {
		animation = requestAnimationFrame(animate);
		return;
	}

	animation = null;
};

/*
retarget recomputes the layout and starts a tween when topology changes.
*/
const retarget = (force: boolean) => {
	const canvas = canvasRef.current;

	if (canvas === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const nextTopology = `${graphVisualKey(graph)}:${Math.round(width)}:${Math.round(height)}`;

	if (!force && nextTopology === topologyKey) {
		return;
	}

	topologyKey = nextTopology;
	targetScene = graph === null ? null : buildScene(graph, width, height);
	fromByIdentity = new Map(renderedByIdentity);
	tweenStart = performance.now();

	if (animation !== null) {
		cancelAnimationFrame(animation);
	}

	animation = requestAnimationFrame(animate);
};

/*
paintThesisEvidence paints the run-wide graph frame, cut to the bound symbol.
*/
const paintThesisEvidence = (value: unknown, focusSymbol: string) => {
	graph = evidenceGraphFor(value, focusSymbol);
	retarget(false);
};

/*
ThesisEvidenceCanvas is the static evidence-graph shell. paintThesis drives it.
*/
export const ThesisEvidenceCanvas = ({ symbol }: { symbol: string }) => {
	const [hover, setHover] = useState<HoverState | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const onResize = () => retarget(true);
		const observer = new ResizeObserver(onResize);
		observer.observe(canvas);
		/*
			The graph frame is the whole run's evidence, published on its own
			cadence. Registering here rather than routing through the modal keeps
			the canvas fed for as long as it is mounted.
		*/
		const unregister = graphStore.subscribe((updates) => {
			paintThesisEvidence(updates, symbol);
		});
		paintThesisEvidence(graphStore.state, symbol);
		retarget(true);

		return () => {
			unregister.unsubscribe();
			observer.disconnect();

			if (animation !== null) {
				cancelAnimationFrame(animation);
				animation = null;
			}
		};
	}, [symbol]);

	const onPointerMove = useCallback(
		(event: React.PointerEvent<HTMLCanvasElement>) => {
			const canvas = canvasRef.current;
			const scene = renderScene;

			if (canvas === null || graph === null || scene === null) {
				return;
			}

			const rect = canvas.getBoundingClientRect();
			const x = event.clientX - rect.left;
			const y = event.clientY - rect.top;
			const hit = hitTest(graph, scene, x, y);
			const nextHoverKey =
				hit !== null && hit.kind === "node" ? hit.node.key : null;

			if (nextHoverKey !== hoverKey) {
				hoverKey = nextHoverKey;

				if (animation === null) {
					draw(nextHoverKey);
				}
			}

			setHover(hit === null ? null : { hit, x, y });
		},
		[],
	);

	const onPointerLeave = useCallback(() => {
		if (hoverKey !== null) {
			hoverKey = null;

			if (animation === null) {
				draw(null);
			}
		}

		setHover(null);
	}, []);

	return (
		<div className="absolute inset-0">
			<canvas
				ref={canvasRef}
				onPointerMove={onPointerMove}
				onPointerLeave={onPointerLeave}
				className="absolute inset-0 block size-full bg-(--sunken)"
			/>
			{hover !== null && (
				<GraphInspector hit={hover.hit} x={hover.x} y={hover.y} />
			)}
		</div>
	);
};