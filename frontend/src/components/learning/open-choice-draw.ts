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
	CONFIRM_MARKS,
	nearPeakFloor,
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
	"#FE81D4",
	"#F6F7D7",
	"#FF9A00",
	"#FF165D",
	"#9FA0FF",
	"#FFF6A9",
	"#D9F9DF",
	"#FFBE91",
	"#C5B3D3",
	"#9EC6F3",
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

export const chartHeld = (pos: Position, book: Book) =>
	onTier(pos, book) && !isClosed(pos);

const markY = (pos: Position): number | undefined =>
	pnlPct(pos) ?? asFinite(pos.pnl_pct) ?? undefined;

const tierOf = (pos: { tier_key?: string }) =>
	typeof pos.tier_key === "string" && pos.tier_key !== ""
		? pos.tier_key
		: "paper";

/** Crest corridor top/bottom while armed — peak ↔ near-peak floor. */
const stopCrest = (stop: Stoploss): { high: number; low: number } => {
	const high = stop.PeakPnLPct;
	const low = nearPeakFloor(stop);
	return high >= low ? { high, low } : { high: low, low: high };
};

/** Fill heat 0..1 from exhaust + confirm progress toward exit. */
const stopHeat = (stop: Stoploss) => {
	if (stop.State === STOPLOSS_TRIGGERED) return 1;
	const confirms = Math.max(0, stop.Confirms ?? 0) / CONFIRM_MARKS;
	const exhaust = Math.max(0, Math.min(1, stop.ExhaustScore ?? 0));
	return Math.max(0, Math.min(1, confirms * 0.55 + exhaust * 0.45));
};

const tipText = (pnl: number | null, stop: Stoploss | undefined) => {
	const money = pnl !== null ? moneyText(pnl) : "";
	if (!stop) return money;
	if (stop.State === STOPLOSS_TRIGGERED) {
		return money ? `${money} · EXIT` : "EXIT";
	}
	if (stop.State !== STOPLOSS_ARMED) return money;
	const c = stop.Confirms ?? 0;
	const exh = stop.ExhaustScore ?? 0;
	const crest = `peak ${stop.PeakPnLPct.toFixed(1)}%`;
	const arm = `ARM c${c}/${CONFIRM_MARKS} exh${exh.toFixed(2)}`;
	return money ? `${money} · ${arm} · ${crest}` : `${arm} · ${crest}`;
};

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
	stroke: string;
	pnlMoney?: number;
	pnlPct?: number;
	stopRoomPct?: number;
	stopProximity?: number;
};

export type OpenChoiceChartApi = {
	sciChartSurface: SciChartSurface;
	setIsGridLayoutMode: (value: boolean) => void;
	setBook: (book: Book) => void;
	push: (
		marks: Record<string, Position>,
		opts?: { snapshot?: boolean; tier?: string },
	) => OpenChoiceLegendItem[];
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
		if (!s) return;
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
		pnl: number | null,
		stop: Stoploss | undefined,
	) => {
		const text = tipText(pnl, stop);
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
		marks: Record<string, Position>,
		opts?: { snapshot?: boolean; tier?: string },
	): OpenChoiceLegendItem[] => {
		if (opts?.snapshot) {
			const tiers = new Set<string>();
			if (opts.tier) tiers.add(opts.tier);
			for (const pos of Object.values(marks)) {
				if (pos) tiers.add(tierOf(pos));
			}
			for (const [id, s] of [...series.entries()]) {
				if (id in marks) continue;
				if (tiers.has(s.tier)) drop(id);
			}
		}

		const x = step++;

		for (const pos of Object.values(marks)) {
			if (!pos) continue;
			const id = pos.id;

			if (!chartHeld(pos, book)) {
				drop(id);
				continue;
			}

			const y = markY(pos);
			if (y === undefined) continue;

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

			tipAt(s, id, x, y, asFinite(pos.pnl), stop);

			legend.set(id, {
				id,
				name: pos.symbol,
				stroke,
				pnlMoney: asFinite(pos.pnl) ?? undefined,
				pnlPct: asFinite(pos.pnl_pct) ?? undefined,
				stopRoomPct: stop ? stopRoom(stop) : undefined,
				stopProximity: stop ? stopProximity(stop) : undefined,
			});
		}

		return [...legend.values()].sort((a, b) => a.name.localeCompare(b.name));
	};

	return { sciChartSurface, setIsGridLayoutMode, setBook, push };
};
