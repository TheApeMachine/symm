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
	XyDataSeries,
	XyyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";
import { GridLayoutModifier } from "./open-choice-grid-layout";
import {
	type OpenChoiceSeries,
	openChoiceSeries,
} from "./open-choice-trail";

let wasmReady = false;

const ensureWasm = () => {
	if (wasmReady) return;
	SciChartSurface.UseCommunityLicense();
	SciChartSurface.loadWasmFromCDN();
	wasmReady = true;
};

const xySeries = (
	wasmContext: ConstructorParameters<typeof XyDataSeries>[0],
	xValues: number[],
	yValues: number[],
	dataSeriesName: string,
) =>
	new XyDataSeries(wasmContext, {
		xValues,
		yValues,
		dataSeriesName,
		isSorted: true,
		containsNaN: false,
	});

const xyySeries = (
	wasmContext: ConstructorParameters<typeof XyyDataSeries>[0],
	xValues: number[],
	yValues: number[],
	y1Values: number[],
	dataSeriesName: string,
) =>
	new XyyDataSeries(wasmContext, {
		xValues,
		yValues,
		y1Values,
		dataSeriesName,
		isSorted: true,
		containsNaN: false,
	});

const sharedX = (values: number[], maxLen: number) => {
	const start = maxLen - values.length;
	return values.map((_, i) => start + i);
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

/** Stable membership key — order must not follow live P&L rank. */
const membershipKey = (drawn: OpenChoiceSeries[]) =>
	drawn
		.map(
			(s) =>
				`${s.id}:${s.dashFrom ?? "-"}:${s.stopFloorTrace?.length ?? 0}:${s.shadowFloorTrace?.length ?? 0}`,
		)
		.sort()
		.join("|");

const ZERO_LINE_ID = "pnl-zero";
const tipAnnotationId = (lotId: string) => `tip:${lotId}`;

const withAlpha = (hex: string, alpha: number) => {
	if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return hex;
	const r = Number.parseInt(hex.slice(1, 3), 16);
	const g = Number.parseInt(hex.slice(3, 5), 16);
	const b = Number.parseInt(hex.slice(5, 7), 16);
	return `rgba(${r},${g},${b},${alpha})`;
};

const moneyText = (value: number) => {
	const abs = Math.abs(value);
	const digits = abs >= 100 ? 0 : abs >= 10 ? 1 : 2;
	// Match the tip callout style: $-0.75 / $+1.25
	return value >= 0 ? `$+${abs.toFixed(digits)}` : `$-${abs.toFixed(digits)}`;
};

const tipStopHint = (item: OpenChoiceSeries) => {
	const proximity = item.stopProximity;
	const room = item.stopRoomPct;
	if (proximity !== undefined && proximity >= 1) return "STOP";
	if (room === undefined) return "";
	const digits = Math.abs(room) >= 10 ? 0 : 1;
	return `${room.toFixed(digits)}pp`;
};

export type OpenChoiceChartApi = {
	sciChartSurface: SciChartSurface;
	setIsGridLayoutMode: (value: boolean) => void;
	syncSeries: (lots: Record<string, unknown>[]) => void;
};

/** SciChart dynamic-layout surface for open-position close-now trails. */
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

	const setIsGridLayoutMode = (value: boolean) => {
		glm.isGrid = value;
	};

	const fillXy = (
		data: XyDataSeries,
		xValues: number[],
		yValues: number[],
	) => {
		data.clear();
		if (xValues.length > 0) data.appendRange(xValues, yValues);
	};

	const fillXyy = (
		data: XyyDataSeries,
		xValues: number[],
		yValues: number[],
		y1Values: number[],
	) => {
		data.clear();
		if (xValues.length > 0) data.appendRange(xValues, yValues, y1Values);
	};

	const seriesByName = () => {
		const map = new Map<string, XyDataSeries | XyyDataSeries>();
		for (const rs of sciChartSurface.renderableSeries.asArray()) {
			const data = rs.dataSeries as XyDataSeries | XyyDataSeries | undefined;
			const name = data?.dataSeriesName;
			if (data && name) map.set(name, data);
		}
		return map;
	};

	let lastKey = "";

	const addLine = (
		id: string,
		xValues: number[],
		yValues: number[],
		stroke: string,
		dashed: boolean,
	) => {
		sciChartSurface.renderableSeries.add(
			new FastLineRenderableSeries(wasmContext, {
				dataSeries: xySeries(wasmContext, xValues, yValues, id),
				stroke: stroke || AUTO_COLOR,
				strokeThickness: 2,
				strokeDashArray: dashed ? [6, 4] : undefined,
			}),
		);
	};

	/*
	Lower-only band: Y is the PnL trail, Y1 is the stop floor. Fill sits between
	them; the upper stroke is transparent so the FastLine keeps the mark path.
	*/
	const addFloorBand = (
		id: string,
		xValues: number[],
		pnlValues: number[],
		floorValues: number[],
		stroke: string,
		dashed: boolean,
		proximity = 0,
	) => {
		if (
			xValues.length === 0 ||
			floorValues.length !== pnlValues.length ||
			pnlValues.length !== xValues.length
		) {
			return;
		}

		const heat = Math.max(0, Math.min(1, proximity));
		const fillAlpha = 0.1 + 0.28 * heat;

		sciChartSurface.renderableSeries.add(
			new FastBandRenderableSeries(wasmContext, {
				dataSeries: xyySeries(
					wasmContext,
					xValues,
					pnlValues,
					floorValues,
					id,
				),
				stroke: "#00000000",
				strokeY1: withAlpha(stroke, dashed ? 0.55 : 0.75 + 0.2 * heat),
				strokeThickness: 1,
				strokeY1DashArray: dashed ? [4, 3] : undefined,
				fill: withAlpha(stroke, fillAlpha),
				fillY1: withAlpha(stroke, fillAlpha),
			}),
		);
	};

	const syncTipLabels = (drawn: OpenChoiceSeries[], maxLen: number) => {
		const keep = new Set<string>([ZERO_LINE_ID]);
		const wanted: NativeTextAnnotation[] = [];

		for (const item of drawn) {
			if (item.values.length === 0 || item.pnlMoney === undefined) continue;

			const xs = sharedX(item.values, maxLen);
			const x = xs[xs.length - 1];
			const y = item.values[item.values.length - 1];
			if (x === undefined || y === undefined) continue;

			const id = tipAnnotationId(item.id);
			keep.add(id);
			const hint = tipStopHint(item);
			wanted.push(
				new NativeTextAnnotation({
					id,
					x1: x,
					y1: y,
					text: hint
						? `${moneyText(item.pnlMoney)}  ${hint}`
						: moneyText(item.pnlMoney),
					fontSize: 12,
					fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
					textColor: item.stroke,
					// Anchor to the right of the tip so the label sits on the trail,
					// inside the plot — Left would clip off the chart edge.
					horizontalAnchorPoint: EHorizontalAnchorPoint.Right,
					verticalAnchorPoint: EVerticalAnchorPoint.Bottom,
				}),
			);
		}

		for (const annotation of [...sciChartSurface.annotations.asArray()]) {
			const id = annotation.id;
			if (typeof id === "string" && id.startsWith("tip:") && !keep.has(id)) {
				sciChartSurface.annotations.remove(annotation);
			}
		}

		for (const next of wanted) {
			const prior = sciChartSurface.annotations
				.asArray()
				.find((annotation) => annotation.id === next.id) as
				| NativeTextAnnotation
				| undefined;

			if (prior) {
				prior.x1 = next.x1;
				prior.y1 = next.y1;
				prior.text = next.text;
				prior.textColor = next.textColor;
			} else {
				sciChartSurface.annotations.add(next);
			}

			// Grid panels hold their own tip clones — keep money in sync there too.
			for (const sub of sciChartSurface.subCharts) {
				const gridTip = sub.annotations
					.asArray()
					.find((annotation) => annotation.id === next.id) as
					| NativeTextAnnotation
					| undefined;
				if (!gridTip) continue;
				gridTip.x1 = next.x1;
				gridTip.y1 = next.y1;
				gridTip.text = next.text;
				gridTip.textColor = next.textColor;
			}
		}
	};

	const rebuild = (drawn: OpenChoiceSeries[], maxLen: number) => {
		const wasGrid = glm.isGrid;
		if (wasGrid) glm.isGrid = false;

		sciChartSurface.renderableSeries.clear(true);

		const ordered = [...drawn].sort((a, b) => a.id.localeCompare(b.id));
		for (const item of ordered) {
			const xs = sharedX(item.values, maxLen);
			const dashFrom = item.dashFrom;

			if (item.stopFloorTrace && item.stopFloorTrace.length === item.values.length) {
				const end =
					dashFrom !== undefined && dashFrom > 0
						? Math.min(dashFrom + 1, item.values.length)
						: item.values.length;
				addFloorBand(
					`${item.id}::stop-band`,
					xs.slice(0, end),
					item.values.slice(0, end),
					item.stopFloorTrace.slice(0, end),
					item.stroke,
					false,
					item.stopProximity ?? 0,
				);
			}

			if (
				item.shadowFloorTrace &&
				item.shadowFloorTrace.length === item.values.length
			) {
				const start =
					dashFrom !== undefined && dashFrom > 0 ? dashFrom : 0;
				addFloorBand(
					`${item.id}::shadow-band`,
					xs.slice(start),
					item.values.slice(start),
					item.shadowFloorTrace.slice(start),
					item.stroke,
					true,
					item.shadowProximity ?? 0,
				);
			}

			if (dashFrom === undefined || dashFrom <= 0 || dashFrom >= item.values.length) {
				addLine(item.id, xs, item.values, item.stroke, false);
				continue;
			}

			addLine(
				item.id,
				xs.slice(0, dashFrom + 1),
				item.values.slice(0, dashFrom + 1),
				item.stroke,
				false,
			);
			addLine(
				`${item.id}::shadow`,
				xs.slice(dashFrom),
				item.values.slice(dashFrom),
				item.stroke,
				true,
			);
		}

		if (wasGrid) glm.isGrid = true;
	};

	const syncSeries = (lots: Record<string, unknown>[]) => {
		const drawn = openChoiceSeries(lots).filter((s) => s.values.length > 0);
		const maxLen = drawn.reduce((n, item) => Math.max(n, item.values.length), 0);
		const nextKey = membershipKey(drawn);

		if (nextKey === lastKey && sciChartSurface.renderableSeries.size() > 0) {
			const byName = seriesByName();
			for (const item of drawn) {
				const xs = sharedX(item.values, maxLen);
				const dashFrom = item.dashFrom;
				const solid = byName.get(item.id) as XyDataSeries | undefined;
				const dashed = byName.get(`${item.id}::shadow`) as
					| XyDataSeries
					| undefined;
				const stopBand = byName.get(`${item.id}::stop-band`) as
					| XyyDataSeries
					| undefined;
				const shadowBand = byName.get(`${item.id}::shadow-band`) as
					| XyyDataSeries
					| undefined;

				if (dashFrom === undefined || dashFrom <= 0 || dashFrom >= item.values.length) {
					if (solid) fillXy(solid, xs, item.values);
					if (dashed) fillXy(dashed, [], []);
				} else {
					if (solid) {
						fillXy(
							solid,
							xs.slice(0, dashFrom + 1),
							item.values.slice(0, dashFrom + 1),
						);
					}
					if (dashed) {
						fillXy(dashed, xs.slice(dashFrom), item.values.slice(dashFrom));
					}
				}

				if (
					stopBand &&
					item.stopFloorTrace &&
					item.stopFloorTrace.length === item.values.length
				) {
					const end =
						dashFrom !== undefined && dashFrom > 0
							? Math.min(dashFrom + 1, item.values.length)
							: item.values.length;
					fillXyy(
						stopBand,
						xs.slice(0, end),
						item.values.slice(0, end),
						item.stopFloorTrace.slice(0, end),
					);
				}

				if (
					shadowBand &&
					item.shadowFloorTrace &&
					item.shadowFloorTrace.length === item.values.length
				) {
					const start =
						dashFrom !== undefined && dashFrom > 0 ? dashFrom : 0;
					fillXyy(
						shadowBand,
						xs.slice(start),
						item.values.slice(start),
						item.shadowFloorTrace.slice(start),
					);
				}
			}
			syncTipLabels(drawn, maxLen);
			return;
		}

		rebuild(drawn, maxLen);
		syncTipLabels(drawn, maxLen);
		lastKey = nextKey;
	};

	return { sciChartSurface, setIsGridLayoutMode, syncSeries };
};
