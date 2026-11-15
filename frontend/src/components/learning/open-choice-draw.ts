import type { Book } from "#/collections/book";
import {
	type Position,
	STOPLOSS_ARMED,
	STOPLOSS_TRIGGERED,
	type Stoploss,
} from "#/collections/types";
import { isClosed, onTier, pnlPct } from "#/components/book-readout";
import { asFinite, moneyText } from "#/lib/format";
import {
	confirmsNeeded,
	givebackFloor,
	stopArmed,
	stopProximity,
	stopRoom,
} from "#/lib/stop-telemetry";
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
import { GridLayoutModifier } from "./open-choice-grid-layout";

/** SciChart fifo capacity — the DataSeries is the trail. */
export const CHOICE_TRAIL_LENGTH = 90;

const ZERO_LINE_ID = "pnl-zero";
const tipId = (positionId: string) => `tip:${positionId}`;

const STROKES = [
	"#79D2C6",
	"#FF7DFD",
	"#5B88FF",
	"#D1765F",
	"#F5F77F",
	"#F8CFF7",
	"#6CA261",
	"#F1B470",
	"#B3A0D4",
	"#80F078",
	"#B4F7EC",
	"#FF8AA8",
	"#B86ED8",
	"#6498C2",
	"#73CA6D",
	"#B3A98B",
	"#73BFFC",
	"#F7D9B1",
	"#E6AAFB",
	"#D1BCC6",
	"#DC6DA0",
	"#CEDB76",
	"#A18B96",
	"#C88BFF",
	"#A9DAF8",
];

const shuffle = <T>(items: readonly T[]): T[] => {
	const out = [...items];
	for (let i = out.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[out[i], out[j]] = [out[j], out[i]];
	}
	return out;
};

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

const withAlpha = (hex: string, alpha: number) => {
	if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return hex;
	const r = Number.parseInt(hex.slice(1, 3), 16);
	const g = Number.parseInt(hex.slice(3, 5), 16);
	const b = Number.parseInt(hex.slice(5, 7), 16);
	return `rgba(${r},${g},${b},${alpha})`;
};

/*
chartHeld is what open-choice paints. Hello only snapshots READY rows, so a
ForceExit Hello drops the id from the store and push() clears series/legend.
*/
export const chartHeld = (pos: Position, book: Book) =>
	onTier(pos, book) && !isClosed(pos);

const markY = (pos: Position): number | undefined =>
	pnlPct(pos) ?? asFinite(pos.pnl_pct) ?? undefined;

const tierOf = (pos: { tier_key?: string }) =>
	typeof pos.tier_key === "string" && pos.tier_key !== ""
		? pos.tier_key
		: "paper";

/** Crest corridor top/bottom while armed — peak ↔ half-climb giveback. */
const stopCrest = (stop: Stoploss): { high: number; low: number } => {
	const high = stop.PeakPnLPct;
	const low = givebackFloor(stop);
	return high >= low ? { high, low } : { high: low, low: high };
};

/** Fill heat 0..1 from exhaust + confirm progress toward exit. */
const stopHeat = (stop: Stoploss) => {
	if (stop.State === STOPLOSS_TRIGGERED) return 1;
	const confirms =
		Math.max(0, stop.Confirms ?? 0) / Math.max(1, confirmsNeeded(stop));
	const exhaust = Math.max(0, Math.min(1, stop.ExhaustScore ?? 0));
	return Math.max(0, Math.min(1, confirms * 0.55 + exhaust * 0.45));
};

const tipText = (net: number | null) =>
	net !== null ? moneyText(net) : "";

type Series = {
	line: FastLineRenderableSeries;
	data: XyDataSeries;
	range?: FastBandRenderableSeries;
	rangeData?: XyyDataSeries;
	stop?: FastBandRenderableSeries;
	stopData?: XyyDataSeries;
	hi: number;
	lo: number;
	tier: string;
	stroke: string;
	tip?: NativeTextAnnotation;
};

export type OpenChoiceLegendItem = {
	id: string;
	name: string;
	tier: string;
	stroke: string;
	pnlMoney?: number;
	pnlPct?: number;
	grossMoney?: number;
	feeMoney?: number;
	entry?: number;
	mark?: number;
	stopRoomPct?: number;
	stopProximity?: number;
};

export type OpenChoiceChartApi = {
	sciChartSurface: SciChartSurface;
	setIsGridLayoutMode: (value: boolean) => void;
	setBook: (book: Book) => void;
	/** Full signals.positions snapshot — drop closed/other-book, append held marks. */
	push: (positions: Record<string, Position>) => OpenChoiceLegendItem[];
};

/** Full positions snapshot → one shared X step, append per open series. */
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
			autoRange: EAutoRange.Always,
		}),
	);
	sciChartSurface.yAxes.add(
		new NumericAxis(wasmContext, {
			axisAlignment: EAxisAlignment.Left,
			growBy: new NumberRange(0.05, 0.05),
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

	const series = new Map<string, Series>();
	const legend = new Map<string, OpenChoiceLegendItem>();
	const strokes = new Map<string, string>();
	const palette = shuffle(STROKES);
	let book: Book = "real";
	let step = 0;

	const setIsGridLayoutMode = (value: boolean) => {
		glm.isGrid = value;
	};

	const strokeFor = (id: string) => {
		const hit = strokes.get(id);
		if (hit) return hit;
		const used = new Set(strokes.values());
		const stroke =
			palette.find((c) => !used.has(c)) ??
			palette[strokes.size % palette.length];
		strokes.set(id, stroke);
		return stroke;
	};

	const drop = (id: string) => {
		const s = series.get(id);
		if (s) {
			sciChartSurface.renderableSeries.remove(s.line);
			s.line.delete();
			if (s.range) {
				sciChartSurface.renderableSeries.remove(s.range);
				s.range.delete();
			}
			if (s.stop) {
				sciChartSurface.renderableSeries.remove(s.stop);
				s.stop.delete();
			}
			if (s.tip) sciChartSurface.annotations.remove(s.tip);
			series.delete(id);
		}
		legend.delete(id);
		strokes.delete(id);
	};

	const setBook = (next: Book) => {
		if (next === book) return;
		book = next;
		for (const id of [...series.keys()]) drop(id);
	};

	const ensure = (
		id: string,
		stroke: string,
		tier: string,
		y: number,
	): Series => {
		const prior = series.get(id);
		if (prior) {
			prior.tier = tier;
			prior.stroke = stroke;
			return prior;
		}

		const data = new XyDataSeries(wasmContext, {
			fifoCapacity: CHOICE_TRAIL_LENGTH,
			dataSeriesName: id,
		});

		const line = new FastLineRenderableSeries(wasmContext, {
			dataSeries: data,
			stroke: stroke || AUTO_COLOR,
			strokeThickness: 2,
		});
		sciChartSurface.renderableSeries.add(line);

		const s: Series = { line, data, hi: y, lo: y, tier, stroke };
		series.set(id, s);
		return s;
	};

	const ensureRange = (s: Series, id: string) => {
		if (s.range && s.rangeData) return;
		const rangeData = new XyyDataSeries(wasmContext, {
			fifoCapacity: CHOICE_TRAIL_LENGTH,
			dataSeriesName: `${id}::range-band`,
		});
		const range = new FastBandRenderableSeries(wasmContext, {
			dataSeries: rangeData,
			stroke: "#00000000",
			strokeY1: "#00000000",
			strokeThickness: 1,
			fill: withAlpha(s.stroke, 0.06),
			fillY1: withAlpha(s.stroke, 0.06),
		});
		sciChartSurface.renderableSeries.insert(0, range);
		s.range = range;
		s.rangeData = rangeData;
	};

	const ensureStop = (s: Series, id: string) => {
		if (s.stop && s.stopData) return;
		const stopData = new XyyDataSeries(wasmContext, {
			fifoCapacity: CHOICE_TRAIL_LENGTH,
			dataSeriesName: `${id}::stop-band`,
		});
		const stop = new FastBandRenderableSeries(wasmContext, {
			dataSeries: stopData,
			stroke: withAlpha(s.stroke, 0.55),
			strokeY1: withAlpha("#e8a33d", 0.7),
			strokeThickness: 1,
			strokeY1DashArray: [4, 3],
			fill: withAlpha(s.stroke, 0.1),
			fillY1: withAlpha("#e8a33d", 0.12),
		});
		// Above the range envelope, under the mark line.
		sciChartSurface.renderableSeries.insert(s.range ? 1 : 0, stop);
		s.stop = stop;
		s.stopData = stopData;
	};

	const paintStop = (s: Series, stop: Stoploss) => {
		if (!s.stop) return;
		const heat = stopHeat(stop);
		const triggered = stop.State === STOPLOSS_TRIGGERED;
		const fillA = triggered ? 0.28 : 0.1 + heat * 0.22;
		const crestA = triggered ? 0.35 : 0.12 + heat * 0.25;
		s.stop.fill = withAlpha(s.stroke, fillA);
		s.stop.fillY1 = withAlpha(triggered ? "#e07a7a" : "#e8a33d", crestA);
		s.stop.stroke = withAlpha(s.stroke, triggered ? 0.85 : 0.45 + heat * 0.4);
		s.stop.strokeY1 = withAlpha(
			triggered ? "#e07a7a" : "#e8a33d",
			triggered ? 0.95 : 0.55 + heat * 0.4,
		);
		s.stop.strokeThickness = triggered ? 2 : 1;
	};

	const tipAt = (
		s: Series,
		id: string,
		x: number,
		y: number,
		net: number | null,
		stop: Stoploss | undefined,
	) => {
		const text = tipText(net);
		if (s.tip) {
			s.tip.x1 = x;
			s.tip.y1 = y;
			s.tip.text = text;
			s.tip.textColor =
				stop?.State === STOPLOSS_TRIGGERED
					? "#e07a7a"
					: stop?.State === STOPLOSS_ARMED
						? "#e8a33d"
						: s.stroke;
			return;
		}
		s.tip = new NativeTextAnnotation({
			id: tipId(id),
			x1: x,
			y1: y,
			text,
			fontSize: 11,
			fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
			textColor: s.stroke,
			padding: Thickness.fromString("0 0 6 0"),
			horizontalAnchorPoint: EHorizontalAnchorPoint.Right,
			verticalAnchorPoint: EVerticalAnchorPoint.Bottom,
		});
		sciChartSurface.annotations.add(s.tip);
	};

	const push = (
		positions: Record<string, Position>,
	): OpenChoiceLegendItem[] => {
		const held = new Set<string>();

		for (const pos of Object.values(positions)) {
			if (pos && chartHeld(pos, book)) held.add(pos.id);
		}

		for (const id of new Set([...series.keys(), ...legend.keys()])) {
			if (!held.has(id)) drop(id);
		}

		const x = step++;

		for (const pos of Object.values(positions)) {
			if (!pos || !held.has(pos.id)) continue;

			const y = markY(pos);
			if (y === undefined) continue;

			const id = pos.id;
			const stroke = strokeFor(id);
			const s = ensure(id, stroke, tierOf(pos), y);
			const stop = pos.stoploss;

			s.hi = Math.max(s.hi, y);
			s.lo = Math.min(s.lo, y);

			s.data.append(x, y);

			ensureRange(s, id);
			s.rangeData?.append(x, s.hi, s.lo);

			if (stop && stopArmed(stop)) {
				ensureStop(s, id);
				const crest = stopCrest(stop);
				s.stopData?.append(x, crest.high, crest.low);
				paintStop(s, stop);
			} else if (s.stopData) {
				s.stopData.append(x, y, y);
				if (s.stop) {
					s.stop.fill = "#00000000";
					s.stop.fillY1 = "#00000000";
					s.stop.stroke = "#00000000";
					s.stop.strokeY1 = "#00000000";
				}
			}

			const feeIn = asFinite(pos.fee_in);
			const feeOut = asFinite(pos.fee_out);
			tipAt(s, id, x, y, asFinite(pos.pnl), stop);

			legend.set(id, {
				id,
				name: pos.symbol,
				tier: tierOf(pos),
				stroke,
				pnlMoney: asFinite(pos.pnl) ?? undefined,
				pnlPct: asFinite(pos.pnl_pct) ?? undefined,
				grossMoney: asFinite(pos.gross_pnl) ?? undefined,
				feeMoney:
					feeIn !== null && feeOut !== null ? feeIn + feeOut : undefined,
				entry: asFinite(pos.entry) ?? undefined,
				mark: asFinite(pos.mark) ?? undefined,
				stopRoomPct: stop ? stopRoom(stop) : undefined,
				stopProximity: stop ? stopProximity(stop) : undefined,
			});
		}

		return [...legend.values()].sort((a, b) => a.name.localeCompare(b.name));
	};

	return { sciChartSurface, setIsGridLayoutMode, setBook, push };
};
