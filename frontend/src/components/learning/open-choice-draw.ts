import {
	AUTO_COLOR,
	EAutoRange,
	EAxisAlignment,
	EHorizontalAnchorPoint,
	ELabelPlacement,
	EVerticalAnchorPoint,
	FastBandRenderableSeries,
	FastLineRenderableSeries,
	HorizontalLineAnnotation,
	MouseWheelZoomModifier,
	NativeTextAnnotation,
	NumberRange,
	NumericAxis,
	SciChartJSDarkTheme,
	SciChartSurface,
	Thickness,
	XyDataSeries,
	XyyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";
import type { Book } from "#/collections/book";
import {
	type Position,
	STOPLOSS_ARMED,
	STOPLOSS_TRIGGERED,
} from "#/collections/types";
import { isClosed, onTier, pnlPct } from "#/components/book-readout";
import { asFinite, moneyText } from "#/lib/format";
import { stopProximity, stopRoom } from "#/lib/stop-telemetry";
import { GridLayoutModifier } from "./open-choice-grid-layout";

/** SciChart fifo capacity — the DataSeries is the trail store. */
export const CHOICE_TRAIL_LENGTH = 90;

const ZERO_LINE_ID = "pnl-zero";
const tipId = (lotId: string) => `tip:${lotId}`;

const STROKES = [
	"#7eb6ff",
	"#f0b429",
	"#5eead4",
	"#c4b5fd",
	"#e07a7a",
	"#86efac",
	"#f9a8d4",
];

let wasmReady = false;

const ensureWasm = () => {
	if (wasmReady) return;

	SciChartSurface.UseCommunityLicense();
	SciChartSurface.loadWasmFromCDN();

	wasmReady = true;
};

const theme = () => {
	const t = new SciChartJSDarkTheme();
	t.sciChartBackground = "#0a0907";
	t.loadingAnimationBackground = "#0a0907";
	t.loadingAnimationForeground = "#e8a33d";
	t.majorGridLineBrush = "#2b251e";
	t.minorGridLineBrush = "#1f1a14";
	t.tickTextBrush = "#938a7e";
	t.labelForegroundBrush = "#cbc2b4";
	t.axisBandsFill = "#00000000";
	t.gridBorderBrush = "#2b251e";
	return t;
};

const strokeFor = (id: string) => {
	let hash = 0;

	for (let i = 0; i < id.length; i++) {
		hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
	}

	return STROKES[hash % STROKES.length];
};

const withAlpha = (hex: string, alpha: number) => {
	if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return hex;
	const r = Number.parseInt(hex.slice(1, 3), 16);
	const g = Number.parseInt(hex.slice(3, 5), 16);
	const b = Number.parseInt(hex.slice(5, 7), 16);
	return `rgba(${r},${g},${b},${alpha})`;
};

/** Open lots on the selected book. */
export const chartHeld = (pos: Position, book: Book) =>
	onTier(pos, book) && !isClosed(pos);

/** Live Y for one append — scalars only; SciChart fifo keeps the trail. */
const markY = (pos: Position): number | undefined =>
	pnlPct(pos) ?? asFinite(pos.pnl_pct) ?? undefined;

const markFloor = (pos: Position): number | undefined => {
	const stop = pos.stoploss;
	if (!stop) return undefined;
	if (stop.State !== STOPLOSS_ARMED && stop.State !== STOPLOSS_TRIGGERED) {
		return undefined;
	}
	return asFinite(stop.FloorPnLPct) ?? undefined;
};

/** Skip duplicate store publishes that did not move this lot's mark. */
const markStamp = (pos: Position): string =>
	[pos.pnl_pct, pos.pnl, pos.mark].join("|");

type LotBind = {
	line: FastLineRenderableSeries;
	data: XyDataSeries;
	band?: FastBandRenderableSeries;
	bandData?: XyyDataSeries;
	stamp: string;
};

export type OpenChoiceLegendItem = {
	id: string;
	name: string;
	stroke: string;
	pnlMoney?: number;
	pnlPct?: number;
	stopRoomPct?: number;
	stopProximity?: number;
};

export type OpenChoiceChartApi = {
	sciChartSurface: SciChartSurface;
	setIsGridLayoutMode: (value: boolean) => void;
	/** Append new marks from a positions store snapshot into SciChart series. */
	sync: (state: Record<string, Position>, book: Book) => OpenChoiceLegendItem[];
};

/** Store subscription → append into fifo DataSeries. Series are the trail. */
export const drawOpenChoiceChart = async (
	rootElement: string | HTMLDivElement,
): Promise<OpenChoiceChartApi> => {
	ensureWasm();

	const { wasmContext, sciChartSurface } = await SciChartSurface.create(
		rootElement,
		{ theme: theme() },
	);

	sciChartSurface.xAxes.add(
		new NumericAxis(wasmContext, {
			drawMinorGridLines: false,
			labelPrecision: 0,
			autoRange: EAutoRange.Always,
		}),
	);

	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			axisAlignment: EAxisAlignment.Left,
			growBy: new NumberRange(0.08, 0.08),
			labelPrecision: 1,
			drawMinorGridLines: false,
			autoRange: EAutoRange.Always,
		}),
	);

	sciChartSurface.annotations.add(
		new HorizontalLineAnnotation({
			id: ZERO_LINE_ID,
			y1: 0,
			stroke: "#5f584e",
			strokeThickness: 1,
			strokeDashArray: [4, 4],
			labelPlacement: ELabelPlacement.Axis,
			axisLabelFill: "#5f584e",
			showLabel: true,
			labelValue: "0",
		}),
	);

	const glm = new GridLayoutModifier();

	sciChartSurface.chartModifiers.add(
		new ZoomExtentsModifier({ modifierGroup: "open-choice" }),
		new MouseWheelZoomModifier({ modifierGroup: "open-choice" }),
		new ZoomPanModifier({ modifierGroup: "open-choice" }),
		glm,
	);

	const lots = new Map<string, LotBind>();
	let nextX = 0;

	const setIsGridLayoutMode = (value: boolean) => {
		glm.isGrid = value;
	};

	const dropLot = (id: string) => {
		const bind = lots.get(id);
		if (!bind) return;

		sciChartSurface.renderableSeries.remove(bind.line);
		bind.line.delete();

		if (bind.band) {
			sciChartSurface.renderableSeries.remove(bind.band);
			bind.band.delete();
		}

		const tip = sciChartSurface.annotations
			.asArray()
			.find((a) => a.id === tipId(id));

		if (tip) sciChartSurface.annotations.remove(tip);

		lots.delete(id);
	};

	const ensureLot = (id: string, stroke: string): LotBind => {
		const prior = lots.get(id);
		if (prior) return prior;

		const data = new XyDataSeries(wasmContext, {
			fifoCapacity: CHOICE_TRAIL_LENGTH,
			dataSeriesName: id,
			dataIsSortedInX: true,
			dataEvenlySpacedInX: true,
			containsNaN: true,
		});

		const line = new FastLineRenderableSeries(wasmContext, {
			dataSeries: data,
			stroke: stroke || AUTO_COLOR,
			strokeThickness: 2,
		});

		sciChartSurface.renderableSeries.add(line);

		const bind: LotBind = { line, data, stamp: "" };
		lots.set(id, bind);

		return bind;
	};

	const ensureBand = (bind: LotBind, id: string, stroke: string) => {
		if (bind.band && bind.bandData) return;

		const bandData = new XyyDataSeries(wasmContext, {
			fifoCapacity: CHOICE_TRAIL_LENGTH,
			dataSeriesName: `${id}::stop-band`,
			dataIsSortedInX: true,
			dataEvenlySpacedInX: true,
			containsNaN: true,
		});

		const band = new FastBandRenderableSeries(wasmContext, {
			dataSeries: bandData,
			stroke: "#00000000",
			strokeY1: withAlpha(stroke, 0.5),
			strokeThickness: 1,
			fill: withAlpha(stroke, 0.08),
			fillY1: withAlpha(stroke, 0.08),
		});

		sciChartSurface.renderableSeries.insert(0, band);

		bind.band = band;
		bind.bandData = bandData;
	};

	const syncTip = (pos: Position, stroke: string, x: number, y: number) => {
		const id = tipId(pos.id);
		const pnl = asFinite(pos.pnl);
		const text = pnl !== null ? moneyText(pnl) : "";

		const prior = sciChartSurface.annotations
			.asArray()
			.find((a) => a.id === id) as NativeTextAnnotation | undefined;

		if (prior) {
			prior.x1 = x;
			prior.y1 = y;
			prior.text = text;
			prior.textColor = stroke;
			prior.padding = Thickness.fromString("0 0 6 0");
			return;
		}

		sciChartSurface.annotations.add(
			new NativeTextAnnotation({
				id,
				x1: x,
				y1: y,
				text,
				fontSize: 12,
				fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
				textColor: stroke,
				// Lift the label a few pixels off the series tip.
				padding: Thickness.fromString("0 0 6 0"),
				horizontalAnchorPoint: EHorizontalAnchorPoint.Right,
				verticalAnchorPoint: EVerticalAnchorPoint.Bottom,
			}),
		);
	};

	const sync = (
		state: Record<string, Position>,
		book: Book,
	): OpenChoiceLegendItem[] => {
		const live = new Set<string>();
		const legend: OpenChoiceLegendItem[] = [];
		const rows: {
			pos: Position;
			bind: LotBind;
			stroke: string;
			y: number;
			floor?: number;
		}[] = [];
		let stepped = false;

		for (const pos of Object.values(state)) {
			if (!chartHeld(pos, book)) continue;

			const id = pos.id;
			live.add(id);

			const y = markY(pos);
			if (y === undefined) continue;

			const stamp = markStamp(pos);
			const stroke = strokeFor(id);
			const bind = ensureLot(id, stroke);
			const floor = markFloor(pos);

			if (floor !== undefined) ensureBand(bind, id, stroke);

			if (stamp !== bind.stamp) {
				stepped = true;
				bind.stamp = stamp;
			}

			rows.push({ pos, bind, stroke, y, floor });

			const stop = pos.stoploss;

			legend.push({
				id,
				name: pos.symbol,
				stroke,
				pnlMoney: asFinite(pos.pnl) ?? undefined,
				pnlPct: asFinite(pos.pnl_pct) ?? undefined,
				stopRoomPct: stop ? stopRoom(stop) : undefined,
				stopProximity: stop ? stopProximity(stop) : undefined,
			});
		}

		if (stepped && rows.length > 0) {
			const x = nextX++;
			for (const { pos, bind, stroke, y, floor } of rows) {
				bind.data.append(x, y);
				if (floor !== undefined && bind.bandData) {
					bind.bandData.append(x, y, floor);
				} else if (bind.bandData) {
					bind.bandData.append(x, y, y);
				}
				syncTip(pos, stroke, x, y);
			}
		}

		for (const id of [...lots.keys()]) {
			if (!live.has(id)) dropLot(id);
		}

		legend.sort((a, b) => Math.abs(b.pnlPct ?? 0) - Math.abs(a.pnlPct ?? 0));
		return legend;
	};

	return { sciChartSurface, setIsGridLayoutMode, sync };
};
