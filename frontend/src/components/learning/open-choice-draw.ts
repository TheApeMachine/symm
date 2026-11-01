import {
	AUTO_COLOR,
	EAutoRange,
	EAxisAlignment,
	ELabelPlacement,
	FastLineRenderableSeries,
	HorizontalLineAnnotation,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	RolloverModifier,
	SciChartJSDarkTheme,
	SciChartSurface,
	XyDataSeries,
	ZoomExtentsModifier,
	ZoomPanModifier,
} from "scichart";
import { GridLayoutModifier } from "./open-choice-grid-layout";
import {
	type OpenChoiceSeries,
	openChoiceSeries,
	openChoiceTrail,
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
		// Ascending sample index, finite P&L only — lets SciChart skip NaN/sort scans.
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
		.map((s) => s.id)
		.sort()
		.join("|");

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
		new RolloverModifier({ modifierGroup: "open-choice" }),
		glm,
	);

	const setIsGridLayoutMode = (value: boolean) => {
		glm.isGrid = value;
	};

	const fillSeries = (
		data: XyDataSeries,
		xValues: number[],
		yValues: number[],
	) => {
		data.clear();
		if (xValues.length > 0) data.appendRange(xValues, yValues);
	};

	const seriesByName = () => {
		const map = new Map<string, XyDataSeries>();
		for (const rs of sciChartSurface.renderableSeries.asArray()) {
			const data = rs.dataSeries as XyDataSeries | undefined;
			const name = data?.dataSeriesName;
			if (data && name) map.set(name, data);
		}
		return map;
	};

	let lastKey = "";

	const rebuild = (drawn: OpenChoiceSeries[], maxLen: number) => {
		const wasGrid = glm.isGrid;
		if (wasGrid) glm.isGrid = false;

		sciChartSurface.renderableSeries.clear(true);

		// Stable insertion order by id so later updates can match by name.
		const ordered = [...drawn].sort((a, b) => a.id.localeCompare(b.id));
		for (const item of ordered) {
			sciChartSurface.renderableSeries.add(
				new FastLineRenderableSeries(wasmContext, {
					dataSeries: xySeries(
						wasmContext,
						sharedX(item.values, maxLen),
						item.values,
						item.id,
					),
					stroke: item.stroke || AUTO_COLOR,
					strokeThickness: 2,
				}),
			);
		}

		if (wasGrid) glm.isGrid = true;
	};

	const syncSeries = (lots: Record<string, unknown>[]) => {
		const drawn = openChoiceSeries(lots).filter((s) => s.values.length > 0);
		const trails = drawn.map((s) => openChoiceTrail(s.id));
		const maxLen = trails.reduce((n, samples) => Math.max(n, samples.length), 0);
		const nextKey = membershipKey(drawn);

		if (nextKey === lastKey && sciChartSurface.renderableSeries.size() > 0) {
			const byName = seriesByName();
			for (const item of drawn) {
				const data = byName.get(item.id);
				if (!data) continue;
				fillSeries(data, sharedX(item.values, maxLen), item.values);
			}
			return;
		}

		rebuild(drawn, maxLen);
		lastKey = nextKey;
	};

	return { sciChartSurface, setIsGridLayoutMode, syncSeries };
};
