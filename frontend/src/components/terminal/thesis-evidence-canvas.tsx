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
import type { Graph } from "#/types/thesis";

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

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value) ? value : value != null ? [value] : []) as T[];

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
paintThesisEvidence paints the current DRAW graphs batch for the bound symbol.
*/
export const paintThesisEvidence = (value: unknown, focusSymbol: string) => {
	const graphs = asRows<Graph>(value);

	graph = graphs.find((frame) => frame.symbol === focusSymbol) ?? null;
	retarget(false);
};

/*
ThesisEvidenceCanvas is the static evidence-graph shell. paintThesis drives it.
*/
export const ThesisEvidenceCanvas = () => {
	const [hover, setHover] = useState<HoverState | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const onResize = () => retarget(true);
		const observer = new ResizeObserver(onResize);
		observer.observe(canvas);
		retarget(true);

		return () => {
			observer.disconnect();

			if (animation !== null) {
				cancelAnimationFrame(animation);
				animation = null;
			}
		};
	}, []);

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
