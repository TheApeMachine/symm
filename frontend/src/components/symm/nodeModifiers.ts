import {
	ChartModifierBase2D,
	EChart2DModifierType,
	ECoordinateMode,
	EHorizontalAnchorPoint,
	EStrokePaletteMode,
	EVerticalAnchorPoint,
	ModifierMouseArgs,
	TextAnnotation,
	translateFromCanvasToSeriesViewRect,
	parseColorToUIntArgb,
	type IPointMarkerPaletteProvider,
	type IRenderableSeries,
	type TPointMarkerArgb,
} from "scichart";

export interface SimNode {
	iata: string;
	label: string;
	x: number;
	y: number;
	vx: number;
	vy: number;
	geoX: number;
	geoY: number;
}

export interface SimEdge {
	sourceIdx: number;
	targetIdx: number;
	weight?: number;
	kind?: "causal" | "backdoor" | "ladder" | "peer";
}

export class EdgeHoverState {
	public hoveredNodeIdx = -1;
}

const TOOLTIP_SNAP_PIXELS = 24;
const LABEL_TEXT_COLOR = "#ffffff";
const LABEL_BACKGROUND_COLOR = "rgba(23,36,61,0.92)";
const LABEL_OFFSET_PIXELS = 20;

export class NodeTooltipModifier extends ChartModifierBase2D {
	public readonly type = EChart2DModifierType.Custom;
	private nodes: SimNode[];
	private edgePalette: EdgeHoverState;
	private adjacency: Set<number>[];
	private pool: TextAnnotation[] = [];
	private lastHoveredIdx = -1;
	private requestRedraw: () => void;

	constructor(
		nodes: SimNode[],
		edges: SimEdge[],
		edgePalette: EdgeHoverState,
		requestRedraw: () => void,
	) {
		super();
		this.nodes = nodes;
		this.edgePalette = edgePalette;
		this.requestRedraw = requestRedraw;

		this.rebuildAdjacency(nodes, edges);
		this.ensureLabelPool(nodes, edges);
	}

	public refreshTopology(nodes: SimNode[], edges: SimEdge[]): void {
		this.nodes = nodes;
		this.rebuildAdjacency(nodes, edges);
		this.ensureLabelPool(nodes, edges);
	}

	private rebuildAdjacency(nodes: SimNode[], edges: SimEdge[]): void {
		this.adjacency = nodes.map(() => new Set<number>());

		for (const edge of edges) {
			if (
				edge.sourceIdx < 0 ||
				edge.targetIdx < 0 ||
				edge.sourceIdx >= nodes.length ||
				edge.targetIdx >= nodes.length
			) {
				continue;
			}

			this.adjacency[edge.sourceIdx].add(edge.targetIdx);
			this.adjacency[edge.targetIdx].add(edge.sourceIdx);
		}
	}

	private ensureLabelPool(nodes: SimNode[], edges: SimEdge[]): void {
		const maxDegree =
			this.adjacency.reduce((max, set) => Math.max(max, set.size), 0) + 1;

		while (this.pool.length < maxDegree) {
			const annotation = new TextAnnotation({
				isHidden: true,
				xCoordinateMode: ECoordinateMode.DataValue,
				yCoordinateMode: ECoordinateMode.DataValue,
				horizontalAnchorPoint: EHorizontalAnchorPoint.Left,
				verticalAnchorPoint: EVerticalAnchorPoint.Bottom,
				textColor: LABEL_TEXT_COLOR,
				fontSize: 14,
				fontFamily: "sans-serif",
				background: LABEL_BACKGROUND_COLOR,
				x1: 0,
				y1: 0,
				text: "",
			});

			this.pool.push(annotation);

			if (this.parentSurface) {
				this.parentSurface.annotations.add(annotation);
			}
		}

		for (; this.pool.length > maxDegree; ) {
			const annotation = this.pool.pop();

			if (annotation && this.parentSurface) {
				this.parentSurface.annotations.remove(annotation);
			}
		}
	}

	public onAttach(): void {
		super.onAttach();
		for (const annotation of this.pool) {
			this.parentSurface.annotations.add(annotation);
		}
	}

	public onDetach(): void {
		for (const annotation of this.pool) {
			this.parentSurface.annotations.remove(annotation);
		}
		super.onDetach();
	}

	private toDataCoords(
		args: ModifierMouseArgs,
	): { x: number; y: number } | null {
		const seriesViewPoint = translateFromCanvasToSeriesViewRect(
			args.mousePoint,
			this.parentSurface.seriesViewRect,
			false,
		);

		if (!seriesViewPoint) {
			return null;
		}

		const xCalc = this.parentSurface.xAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const yCalc = this.parentSurface.yAxes
			.get(0)
			.getCurrentCoordinateCalculator();

		return {
			x: xCalc.getDataValue(seriesViewPoint.x),
			y: yCalc.getDataValue(seriesViewPoint.y),
		};
	}

	private showLabels(hoveredIdx: number): void {
		const xCalc = this.parentSurface.xAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const yCalc = this.parentSurface.yAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const offsetX = Math.abs(
			xCalc.getDataValue(LABEL_OFFSET_PIXELS) - xCalc.getDataValue(0),
		);
		const offsetY = Math.abs(
			yCalc.getDataValue(0) - yCalc.getDataValue(LABEL_OFFSET_PIXELS),
		);
		const toLabel = [hoveredIdx, ...this.adjacency[hoveredIdx]];
		let poolIdx = 0;

		for (const nodeIdx of toLabel) {
			const node = this.nodes[nodeIdx];
			const annotation = this.pool[poolIdx];
			poolIdx += 1;
			annotation.text = node.label;
			annotation.x1 = node.x + offsetX;
			annotation.y1 = node.y + offsetY;
			annotation.isHidden = false;
		}

		for (; poolIdx < this.pool.length; poolIdx += 1) {
			if (!this.pool[poolIdx].isHidden) {
				this.pool[poolIdx].isHidden = true;
			}
		}
	}

	private hideAll(): void {
		for (const annotation of this.pool) {
			if (!annotation.isHidden) {
				annotation.isHidden = true;
			}
		}
	}

	private hitTestNode(args: ModifierMouseArgs): void {
		const point = this.toDataCoords(args);

		if (!point) {
			this.edgePalette.hoveredNodeIdx = -1;
			this.hideAll();
			return;
		}

		const xCalc = this.parentSurface.xAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const snapDataUnits = Math.abs(
			xCalc.getDataValue(TOOLTIP_SNAP_PIXELS) - xCalc.getDataValue(0),
		);

		let closestIdx = -1;
		let minDist = Infinity;

		for (let index = 0; index < this.nodes.length; index += 1) {
			const deltaX = this.nodes[index].x - point.x;
			const deltaY = this.nodes[index].y - point.y;
			const dist = Math.sqrt(deltaX * deltaX + deltaY * deltaY);

			if (dist < minDist) {
				minDist = dist;
				closestIdx = index;
			}
		}

		if (closestIdx >= 0 && minDist <= snapDataUnits) {
			if (closestIdx !== this.lastHoveredIdx) {
				this.edgePalette.hoveredNodeIdx = closestIdx;
				this.lastHoveredIdx = closestIdx;
				this.requestRedraw();
			}
			this.showLabels(closestIdx);
			return;
		}

		if (this.lastHoveredIdx !== -1) {
			this.edgePalette.hoveredNodeIdx = -1;
			this.hideAll();
			this.lastHoveredIdx = -1;
			this.requestRedraw();
		}
	}

	public modifierMouseDown(args: ModifierMouseArgs): void {
		super.modifierMouseDown(args);
		this.hitTestNode(args);
	}

	public modifierMouseMove(args: ModifierMouseArgs): void {
		super.modifierMouseMove(args);
		this.hitTestNode(args);
	}

	public modifierMouseUp(args: ModifierMouseArgs): void {
		super.modifierMouseUp(args);

		if (this.lastHoveredIdx !== -1) {
			this.edgePalette.hoveredNodeIdx = -1;
			this.hideAll();
			this.lastHoveredIdx = -1;
			this.requestRedraw();
		}
	}

	public modifierMouseWheel(args: ModifierMouseArgs): void {
		super.modifierMouseWheel(args);

		if (this.lastHoveredIdx !== -1) {
			this.showLabels(this.lastHoveredIdx);
		}
	}

	public modifierMouseLeave(args: ModifierMouseArgs): void {
		super.modifierMouseLeave(args);

		if (this.lastHoveredIdx !== -1) {
			this.edgePalette.hoveredNodeIdx = -1;
			this.hideAll();
			this.lastHoveredIdx = -1;
			this.requestRedraw();
		}
	}
}

interface DragState {
	nodeIdx: number;
	dataX: number;
	dataY: number;
}

export type DragStateRef = { current: DragState | null };

const DRAG_SNAP_PIXELS = 24;
const DRAG_THRESHOLD_PIXELS = 6;

export class NodeDragModifier extends ChartModifierBase2D {
	public readonly type = EChart2DModifierType.Custom;
	private nodes: SimNode[];
	private dragState: DragStateRef;
	private reheat: () => void;
	private pendingNodeIdx = -1;
	private downPoint: { x: number; y: number } | null = null;

	constructor(nodes: SimNode[], dragState: DragStateRef, reheat: () => void) {
		super();
		this.nodes = nodes;
		this.dragState = dragState;
		this.reheat = reheat;
	}

	private toDataCoords(
		args: ModifierMouseArgs,
	): { x: number; y: number } | null {
		const seriesViewPoint = translateFromCanvasToSeriesViewRect(
			args.mousePoint,
			this.parentSurface.seriesViewRect,
			false,
		);

		if (!seriesViewPoint) {
			return null;
		}

		const xCalc = this.parentSurface.xAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const yCalc = this.parentSurface.yAxes
			.get(0)
			.getCurrentCoordinateCalculator();

		return {
			x: xCalc.getDataValue(seriesViewPoint.x),
			y: yCalc.getDataValue(seriesViewPoint.y),
		};
	}

	public modifierMouseDown(args: ModifierMouseArgs): void {
		super.modifierMouseDown(args);
		const point = this.toDataCoords(args);

		if (!point) {
			return;
		}

		let closestIdx = -1;
		let minDist = Infinity;

		for (let index = 0; index < this.nodes.length; index += 1) {
			const deltaX = this.nodes[index].x - point.x;
			const deltaY = this.nodes[index].y - point.y;
			const dist = Math.sqrt(deltaX * deltaX + deltaY * deltaY);

			if (dist < minDist) {
				minDist = dist;
				closestIdx = index;
			}
		}

		const xCalc = this.parentSurface.xAxes
			.get(0)
			.getCurrentCoordinateCalculator();
		const snapDataUnits = Math.abs(
			xCalc.getDataValue(DRAG_SNAP_PIXELS) - xCalc.getDataValue(0),
		);

		if (closestIdx >= 0 && minDist <= snapDataUnits) {
			this.pendingNodeIdx = closestIdx;
			this.downPoint = { x: args.mousePoint.x, y: args.mousePoint.y };
			args.handled = true;
		}
	}

	public modifierMouseMove(args: ModifierMouseArgs): void {
		super.modifierMouseMove(args);

		if (this.pendingNodeIdx >= 0 && !this.dragState.current && this.downPoint) {
			const deltaX = args.mousePoint.x - this.downPoint.x;
			const deltaY = args.mousePoint.y - this.downPoint.y;
			const dist = Math.sqrt(deltaX * deltaX + deltaY * deltaY);

			if (dist >= DRAG_THRESHOLD_PIXELS) {
				const point = this.toDataCoords(args);

				if (point) {
					this.dragState.current = {
						nodeIdx: this.pendingNodeIdx,
						dataX: point.x,
						dataY: point.y,
					};
					this.reheat();
				}

				this.downPoint = null;
			}

			args.handled = true;
			return;
		}

		if (!this.dragState.current) {
			return;
		}

		const point = this.toDataCoords(args);

		if (!point) {
			return;
		}

		this.dragState.current.dataX = point.x;
		this.dragState.current.dataY = point.y;
		args.handled = true;
	}

	public modifierMouseUp(args: ModifierMouseArgs): void {
		super.modifierMouseUp(args);
		this.pendingNodeIdx = -1;
		this.downPoint = null;

		if (!this.dragState.current) {
			return;
		}

		this.dragState.current = null;
		this.reheat();
		args.handled = true;
	}
}

const HOVER_FILL = parseColorToUIntArgb("#47bde6");
const HOVER_STROKE = parseColorToUIntArgb("#274b92");

export class NodeHoverPaletteProvider implements IPointMarkerPaletteProvider {
	public readonly strokePaletteMode = EStrokePaletteMode.SOLID;
	private edgeHover: EdgeHoverState;

	constructor(edgeHover: EdgeHoverState) {
		this.edgeHover = edgeHover;
	}

	public onAttached(_parentSeries: IRenderableSeries): void {}

	public onDetached(): void {}

	public overridePointMarkerArgb(
		_xValue: number,
		_yValue: number,
		index: number,
	): TPointMarkerArgb | undefined {
		if (index === this.edgeHover.hoveredNodeIdx) {
			return { fill: HOVER_FILL, stroke: HOVER_STROKE };
		}

		return undefined;
	}
}
