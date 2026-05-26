import {
	EAutoRange,
	EllipsePointMarker,
	FastLineSegmentRenderableSeries,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	PinchZoomModifier,
	SciChartSurface,
	XyDataSeries,
	XyScatterRenderableSeries,
	XyxyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";

import {
	buildGraphState,
	replaceGraphNodes,
} from "#/components/symm/causal-graph-layout";
import { type CausalGraphRow } from "#/components/symm/causal-graph-data-provider";
import {
	type DragStateRef,
	EdgeHoverState,
	NodeDragModifier,
	NodeHoverPaletteProvider,
	NodeTooltipModifier,
	type SimEdge,
	type SimNode,
} from "#/components/symm/nodeModifiers";
import { appTheme } from "#/components/symm/theme";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const REPULSION_STRENGTH = -120;
const REPULSION_MIN_DIST = 1;
const SPRING_K = 0.3;
const SPRING_REST_LENGTH = 20;
const GEO_ANCHOR_STRENGTH = 0.12;
const VELOCITY_DECAY = 0.6;

type EdgeKind = "causal" | "backdoor" | "ladder" | "peer";

const tick = (nodes: SimNode[], edges: SimEdge[], alpha: number): void => {
	for (let left = 0; left < nodes.length; left += 1) {
		for (let right = left + 1; right < nodes.length; right += 1) {
			const deltaX = nodes[right].x - nodes[left].x;
			const deltaY = nodes[right].y - nodes[left].y;
			const distance = Math.max(
				Math.sqrt(deltaX * deltaX + deltaY * deltaY),
				REPULSION_MIN_DIST,
			);
			const force = (REPULSION_STRENGTH * alpha) / (distance * distance);
			const forceX = force * (deltaX / distance);
			const forceY = force * (deltaY / distance);
			nodes[left].vx += forceX;
			nodes[left].vy += forceY;
			nodes[right].vx -= forceX;
			nodes[right].vy -= forceY;
		}
	}

	for (const edge of edges) {
		const source = nodes[edge.sourceIdx];
		const target = nodes[edge.targetIdx];
		const deltaX = target.x + target.vx - (source.x + source.vx);
		const deltaY = target.y + target.vy - (source.y + source.vy);
		const distance = Math.max(Math.sqrt(deltaX * deltaX + deltaY * deltaY), 1);
		const rest =
			SPRING_REST_LENGTH + Math.min(12, Math.abs(edge.weight ?? 0) * 4);
		const force = SPRING_K * (distance - rest) * alpha;
		const forceX = force * (deltaX / distance);
		const forceY = force * (deltaY / distance);
		source.vx += forceX * 0.5;
		source.vy += forceY * 0.5;
		target.vx -= forceX * 0.5;
		target.vy -= forceY * 0.5;
	}

	for (const node of nodes) {
		node.vx += (node.geoX - node.x) * GEO_ANCHOR_STRENGTH * alpha;
		node.vy += (node.geoY - node.y) * GEO_ANCHOR_STRENGTH * alpha;
	}

	for (const node of nodes) {
		node.vx *= VELOCITY_DECAY;
		node.vy *= VELOCITY_DECAY;
		node.x += node.vx;
		node.y += node.vy;
	}
};

export type CausalForceGraphControls = {
	update: (row: CausalGraphRow) => void;
	dispose: () => void;
};

export const initCausalForceGraph = async (
	rootElement: string | HTMLDivElement,
): Promise<{
	sciChartSurface: SciChartSurface;
	controls: CausalForceGraphControls;
}> => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
		},
	);

	const xAxis = new NumericAxis(wasmContext, {
		isVisible: false,
		autoRange: EAutoRange.Never,
		visibleRangeLimit: new NumberRange(-600, 600),
	});
	const yAxis = new NumericAxis(wasmContext, {
		isVisible: false,
		autoRange: EAutoRange.Never,
		visibleRangeLimit: new NumberRange(-600, 600),
	});
	xAxis.visibleRange = new NumberRange(-220, 220);
	yAxis.visibleRange = new NumberRange(-180, 180);
	sciChartSurface.xAxes.add(xAxis);
	sciChartSurface.yAxes.add(yAxis);

	const initial = buildGraphState(undefined);
	const nodes: SimNode[] = [...initial.nodes];
	let edges: SimEdge[] = initial.edges;
	const edgeHover = new EdgeHoverState();

	const edgeSeriesByKind = new Map<EdgeKind, XyxyDataSeries>();
	const edgeStyles: Record<
		EdgeKind,
		{ stroke: string; strokeThickness: number }
	> = {
		causal: { stroke: "#47bde650", strokeThickness: 2 },
		backdoor: { stroke: "#88888866", strokeThickness: 1.5 },
		ladder: { stroke: "#a78bfa88", strokeThickness: 2 },
		peer: { stroke: "#f59e0b77", strokeThickness: 1.75 },
	};

	for (const kind of Object.keys(edgeStyles) as EdgeKind[]) {
		const dataSeries = new XyxyDataSeries(wasmContext);
		edgeSeriesByKind.set(kind, dataSeries);
		sciChartSurface.renderableSeries.add(
			new FastLineSegmentRenderableSeries(wasmContext, {
				dataSeries,
				stroke: edgeStyles[kind].stroke,
				strokeThickness: edgeStyles[kind].strokeThickness,
			}),
		);
	}

	const edgeHighlightDataSeries = new XyxyDataSeries(wasmContext);
	sciChartSurface.renderableSeries.add(
		new FastLineSegmentRenderableSeries(wasmContext, {
			dataSeries: edgeHighlightDataSeries,
			stroke: "#47bde6",
			strokeThickness: 3,
		}),
	);

	const nodeDataSeries = new XyDataSeries(wasmContext);
	sciChartSurface.renderableSeries.add(
		new XyScatterRenderableSeries(wasmContext, {
			dataSeries: nodeDataSeries,
			pointMarker: new EllipsePointMarker(wasmContext, {
				width: 16,
				height: 16,
				fill: "#274b92",
				stroke: "#47bde6",
				strokeThickness: 1.5,
			}),
			paletteProvider: new NodeHoverPaletteProvider(edgeHover),
		}),
	);

	const dragState: DragStateRef = { current: null };

	let alpha = 1;
	let running = true;
	let loopAlive = false;
	let autoZoomed = false;
	let animFrameId = 0;

	const appendEdges = (edgeList: SimEdge[], kind: EdgeKind) => {
		const target = edgeSeriesByKind.get(kind);

		if (!target) {
			return;
		}

		const xs: number[] = [];
		const ys: number[] = [];
		const x1s: number[] = [];
		const y1s: number[] = [];

		for (const edge of edgeList) {
			if (edge.kind !== kind) {
				continue;
			}

			const source = nodes[edge.sourceIdx];
			const targetNode = nodes[edge.targetIdx];

			if (!source || !targetNode) {
				continue;
			}

			xs.push(source.x);
			ys.push(source.y);
			x1s.push(targetNode.x);
			y1s.push(targetNode.y);
		}

		target.clear();
		target.appendRange(xs, ys, x1s, y1s);
	};

	const frame = () => {
		if (!running || sciChartSurface.isDeleted) {
			loopAlive = false;
			return;
		}

		const simActive = alpha >= 0.001 || dragState.current !== null;

		if (simActive) {
			tick(nodes, edges, alpha);
			alpha *= 0.9772;

			if (!autoZoomed && alpha < 0.5) {
				autoZoomed = true;
				sciChartSurface.zoomExtents(200);
			}

			if (dragState.current) {
				const dragged = nodes[dragState.current.nodeIdx];

				if (dragged) {
					dragged.x = dragState.current.dataX;
					dragged.y = dragState.current.dataY;
					dragged.vx = 0;
					dragged.vy = 0;
				}

				alpha = Math.max(alpha, 0.1);
			}
		}

		const hovered = edgeHover.hoveredNodeIdx;
		const highlightXs: number[] = [];
		const highlightYs: number[] = [];
		const highlightX1s: number[] = [];
		const highlightY1s: number[] = [];

		if (hovered !== -1) {
			for (const edge of edges) {
				if (edge.sourceIdx !== hovered && edge.targetIdx !== hovered) {
					continue;
				}

				const source = nodes[edge.sourceIdx];
				const targetNode = nodes[edge.targetIdx];

				if (!source || !targetNode) {
					continue;
				}

				highlightXs.push(source.x);
				highlightYs.push(source.y);
				highlightX1s.push(targetNode.x);
				highlightY1s.push(targetNode.y);
			}
		}

		for (const kind of Object.keys(edgeStyles) as EdgeKind[]) {
			appendEdges(edges, kind);
		}

		edgeHighlightDataSeries.clear();
		edgeHighlightDataSeries.appendRange(
			highlightXs,
			highlightYs,
			highlightX1s,
			highlightY1s,
		);

		nodeDataSeries.clear();
		nodeDataSeries.appendRange(
			nodes.map((node) => node.x),
			nodes.map((node) => node.y),
		);

		if (simActive) {
			animFrameId = requestAnimationFrame(frame);
		} else {
			loopAlive = false;
		}
	};

	const startLoop = () => {
		if (!loopAlive) {
			loopAlive = true;
			animFrameId = requestAnimationFrame(frame);
		}
	};

	const tooltipModifier = new NodeTooltipModifier(nodes, edges, edgeHover, () =>
		startLoop(),
	);

	sciChartSurface.chartModifiers.add(
		tooltipModifier,
		new NodeDragModifier(nodes, dragState, () => {
			alpha = Math.max(alpha, 0.3);
			startLoop();
		}),
		new ZoomPanModifier(),
		new ZoomExtentsModifier(),
		new MouseWheelZoomModifier(),
		new PinchZoomModifier(),
	);

	startLoop();

	return {
		sciChartSurface,
		controls: {
			update(row: CausalGraphRow) {
				if (sciChartSurface.isDeleted) {
					return;
				}

				const rebuilt = buildGraphState(row);
				replaceGraphNodes(nodes, rebuilt.nodes);
				edges = rebuilt.edges;
				tooltipModifier.refreshTopology(nodes, edges);
				alpha = Math.max(alpha, 0.35);
				startLoop();
			},
			dispose() {
				running = false;

				if (animFrameId) {
					cancelAnimationFrame(animFrameId);
					animFrameId = 0;
				}
			},
		},
	};
};

export const drawExample = initCausalForceGraph;
