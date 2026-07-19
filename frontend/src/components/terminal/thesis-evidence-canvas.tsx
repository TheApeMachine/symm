import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useRef, useState } from "react";
import { appStore } from "#/collections/app";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";
import type { GraphFrame } from "#/types/thesis";

type HoverState = {
	hit: GraphHit;
	x: number;
	y: number;
};

const TWEEN_MS = 420;

const easeInOut = (t: number): number =>
	t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2;

/*
ThesisEvidenceCanvas paints the symbol's evidence graph and resolves pointer
hover into a node/edge inspector. Layout targets are recomputed per frame from
stable node identity, and rendered positions are eased toward those targets so a
tick that adds or drops nodes glides instead of snapping — the graph reads as one
evolving structure rather than a fresh scatter each tick.
*/
export const ThesisEvidenceCanvas = ({ symbol }: { symbol: string }) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const graphRef = useRef<GraphFrame | null>(null);
	const targetSceneRef = useRef<GraphScene | null>(null);
	const renderSceneRef = useRef<GraphScene | null>(null);
	// Rendered positions carried across frames by stable identity, so tweening
	// survives the per-tick MeasurementKey churn.
	const renderedByIdentityRef = useRef<Map<string, GraphNodePosition>>(new Map());
	const topologyKeyRef = useRef("");
	const hoverKeyRef = useRef<string | null>(null);
	const animationRef = useRef<number | null>(null);
	const tweenStartRef = useRef(0);
	const fromByIdentityRef = useRef<Map<string, GraphNodePosition>>(new Map());
	const online = useSelector(appStore, (state) => state.online);
	const [hover, setHover] = useState<HoverState | null>(null);

	// Draws the render scene (already-interpolated positions) to the canvas.
	const draw = useCallback((hoverKey: string | null) => {
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

		drawEvidenceGraph(
			context,
			width,
			height,
			graphRef.current,
			renderSceneRef.current,
			hoverKey,
		);
	}, []);

	// Builds the render scene at tween progress p (0..1) from the remembered
	// origin positions toward the target layout, keyed by node identity so nodes
	// keep continuity even as their per-tick keys change.
	const composeRenderScene = useCallback((progress: number) => {
		const target = targetSceneRef.current;

		if (target === null) {
			renderSceneRef.current = null;

			return;
		}

		const positions = new Map<string, GraphNodePosition>();
		const rendered = new Map<string, GraphNodePosition>();
		const eased = easeInOut(progress);

		for (const [key, node] of target.nodes) {
			const identity = nodeIdentity(node);
			const to = target.positions.get(key);

			if (to === undefined) {
				continue;
			}

			const from = fromByIdentityRef.current.get(identity) ?? to;
			const position = {
				x: from.x + (to.x - from.x) * eased,
				y: from.y + (to.y - from.y) * eased,
			};

			positions.set(key, position);
			rendered.set(identity, position);
		}

		renderSceneRef.current = {
			positions,
			nodes: target.nodes,
			width: target.width,
			height: target.height,
		};
		renderedByIdentityRef.current = rendered;
	}, []);

	const animate = useCallback(
		(now: number) => {
			const elapsed = now - tweenStartRef.current;
			const progress = Math.min(1, elapsed / TWEEN_MS);

			composeRenderScene(progress);
			draw(hoverKeyRef.current);

			if (progress < 1) {
				animationRef.current = requestAnimationFrame(animate);
			} else {
				animationRef.current = null;
			}
		},
		[composeRenderScene, draw],
	);

	// Recomputes the target layout and, if topology changed, starts a tween from
	// the currently rendered positions to the new target.
	const retarget = useCallback(
		(force: boolean) => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const width = canvas.clientWidth;
			const height = canvas.clientHeight;
			const graph = graphRef.current;
			const nextTopology = `${graphVisualKey(graph)}:${Math.round(width)}:${Math.round(height)}`;

			if (!force && nextTopology === topologyKeyRef.current) {
				return;
			}

			topologyKeyRef.current = nextTopology;
			targetSceneRef.current =
				graph === null ? null : buildScene(graph, width, height);

			// Origin = where each identity is rendered right now (empty on first
			// frame, so new nodes simply appear at their target).
			fromByIdentityRef.current = new Map(renderedByIdentityRef.current);
			tweenStartRef.current = performance.now();

			if (animationRef.current !== null) {
				cancelAnimationFrame(animationRef.current);
			}

			animationRef.current = requestAnimationFrame(animate);
		},
		[animate],
	);

	useDirectStorePaint(
		getWorker(),
		[{ store: "graphs", key: symbol }],
		(buffers) => {
			const graphs = (buffers[`graphs:${symbol}`] ??
				buffers["graphs:"] ??
				[]) as GraphFrame[];

			graphRef.current =
				graphs.find((frame) => frame.symbol === symbol) ?? null;
			retarget(false);
		},
		[symbol, online, retarget],
	);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const onResize = () => retarget(true);
		const observer = new ResizeObserver(onResize);
		observer.observe(canvas);

		return () => {
			observer.disconnect();

			if (animationRef.current !== null) {
				cancelAnimationFrame(animationRef.current);
			}
		};
	}, [retarget]);

	const onPointerMove = useCallback(
		(event: React.PointerEvent<HTMLCanvasElement>) => {
			const canvas = canvasRef.current;
			const graph = graphRef.current;
			const scene = renderSceneRef.current;

			if (canvas === null || graph === null || scene === null) {
				return;
			}

			const rect = canvas.getBoundingClientRect();
			const x = event.clientX - rect.left;
			const y = event.clientY - rect.top;
			const hit = hitTest(graph, scene, x, y);
			const nextHoverKey =
				hit !== null && hit.kind === "node" ? hit.node.key : null;

			if (nextHoverKey !== hoverKeyRef.current) {
				hoverKeyRef.current = nextHoverKey;

				// Only force an immediate redraw when no tween is in flight; the
				// animation loop already repaints with the current hover key.
				if (animationRef.current === null) {
					draw(nextHoverKey);
				}
			}

			setHover(hit === null ? null : { hit, x, y });
		},
		[draw],
	);

	const onPointerLeave = useCallback(() => {
		if (hoverKeyRef.current !== null) {
			hoverKeyRef.current = null;

			if (animationRef.current === null) {
				draw(null);
			}
		}

		setHover(null);
	}, [draw]);

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
