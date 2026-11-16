import {
	EAutoRange,
	EAxisAlignment,
	ELabelPlacement,
	HorizontalLineAnnotation,
	MouseWheelZoomModifier,
	NumberRange,
	NumericAxis,
	SciChartJSDarkTheme,
	SciChartSurface,
	ZoomExtentsModifier,
	ZoomPanModifier,
	type TSciChart,
} from "scichart";
import { GridLayoutModifier } from "./open-choice-grid-layout";

/** SciChart fifo capacity — the DataSeries is the trail. */
export const CHOICE_TRAIL_LENGTH = 90;

const ZERO_LINE_ID = "pnl-zero";

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

export type OpenChoiceChartApi = {
	wasmContext: TSciChart;
	sciChartSurface: SciChartSurface;
	setIsGridLayoutMode: (value: boolean) => void;
};

/** Surface + one line series per position id. Append lives in app.paintOpenChoices. */
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

	return {
		wasmContext,
		sciChartSurface,
		setIsGridLayoutMode: (value: boolean) => {
			glm.isGrid = value;
		},
	};
};
