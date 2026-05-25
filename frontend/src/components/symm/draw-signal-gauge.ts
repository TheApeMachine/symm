import {
	EAxisAlignment,
	ECoordinateMode,
	EHorizontalAnchorPoint,
	EPolarAxisMode,
	EPolarLabelMode,
	EStrokeLineJoin,
	EVerticalAnchorPoint,
	NativeTextAnnotation,
	NumberRange,
	PolarArcAnnotation,
	PolarNumericAxis,
	PolarPointerAnnotation,
	SciChartPolarSurface,
	Thickness,
} from "scichart";

import { appTheme } from "#/components/symm/theme";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";
import { formatSignalConfidence } from "#/lib/symm/signal-confidence";

const GAUGE_BANDS = [50, 75, 100] as const;
const GRADIENT_COLORS = [
	appTheme.VividGreen,
	appTheme.VividOrange,
	appTheme.VividPink,
] as const;

type GaugeArcSet = {
	valueArcs: PolarArcAnnotation[];
	pointer: PolarPointerAnnotation;
	label: NativeTextAnnotation;
};

const applyGaugeNeedle = (
	gaugeArcs: GaugeArcSet,
	needlePercent: number,
): void => {
	const pointerValue = Math.max(0, Math.min(100, needlePercent));
	let hasPointerPassedValue = false;

	gaugeArcs.pointer.x1 = pointerValue;

	for (let index = 0; index < gaugeArcs.valueArcs.length; index += 1) {
		const bandTop = GAUGE_BANDS[index];
		const bandBottom = GAUGE_BANDS[index - 1] ?? 0;
		const arcEnd = hasPointerPassedValue
			? bandBottom
			: bandTop > pointerValue
				? pointerValue
				: bandTop;

		gaugeArcs.valueArcs[index].y1 = bandBottom;
		gaugeArcs.valueArcs[index].y2 = arcEnd;

		if (bandTop >= pointerValue) {
			hasPointerPassedValue = true;
		}
	}
};

const buildGaugeArcs = (
	sciChartSurface: SciChartPolarSurface,
	pointerValue: number,
): GaugeArcSet => {
	const valueArcs: PolarArcAnnotation[] = [];
	let hasPointerPassedValue = false;

	for (let index = 0; index < GAUGE_BANDS.length; index += 1) {
		const bandTop = GAUGE_BANDS[index];
		const bandBottom = GAUGE_BANDS[index - 1] ?? 0;

		sciChartSurface.annotations.add(
			new PolarArcAnnotation({
				x2: 7.6,
				x1: 7.9,
				y1: bandBottom,
				y2: bandTop,
				fill: GRADIENT_COLORS[index],
				strokeThickness: 0,
			}),
		);

		const valueArc = new PolarArcAnnotation({
			id: `arc${index}`,
			x2: 8.1,
			x1: 10,
			y1: bandBottom,
			y2: hasPointerPassedValue
				? bandBottom
				: bandTop > pointerValue
					? pointerValue
					: bandTop,
			fill: GRADIENT_COLORS[index],
			strokeThickness: 0,
		});

		valueArcs.push(valueArc);

		sciChartSurface.annotations.add(valueArc);

		if (bandTop >= pointerValue) {
			hasPointerPassedValue = true;
		}
	}

	const pointer = new PolarPointerAnnotation({
		x1: pointerValue,
		y1: 7.6,
		xCoordinateMode: ECoordinateMode.DataValue,
		yCoordinateMode: ECoordinateMode.DataValue,
		pointerStyle: {
			baseSize: 0,
			strokeWidth: 0,
		},
		pointerArrowStyle: {
			strokeWidth: 2,
			stroke: "white",
			fill: "none",
			height: 0.4,
			width: 0.25,
		},
		strokeLineJoin: EStrokeLineJoin.Miter,
	});

	const label = new NativeTextAnnotation({
		text: formatSignalConfidence(0),
		x1: 0,
		y1: 0,
		textColor: "#FFFFFF",
		fontSize: 16,
		padding: new Thickness(0, 0, 16, 0),
		xCoordinateMode: ECoordinateMode.DataValue,
		yCoordinateMode: ECoordinateMode.DataValue,
		verticalAnchorPoint: EVerticalAnchorPoint.Center,
		horizontalAnchorPoint: EHorizontalAnchorPoint.Center,
	});

	sciChartSurface.annotations.add(pointer, label);

	return { valueArcs, pointer, label };
};

export type SignalGaugeControls = {
	update: (needlePercent: number, confidence: number) => void;
	dispose: () => void;
};

export const drawSignalGauge = async (rootElement: HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChartSurface, wasmContext } = await SciChartPolarSurface.create(
		rootElement,
		{
			padding: new Thickness(0, 0, 0, 0),
			background: appTheme.Background,
		},
	);

	sciChartSurface.xAxes.add(
		new PolarNumericAxis(wasmContext, {
			polarAxisMode: EPolarAxisMode.Radial,
			axisAlignment: EAxisAlignment.Right,
			startAngle: (Math.PI * 3) / 2 + Math.PI / 4,
			drawLabels: false,
			drawMinorGridLines: false,
			drawMajorGridLines: false,
			drawMajorTickLines: false,
			drawMinorTickLines: false,
		}),
	);

	sciChartSurface.yAxes.add(
		new PolarNumericAxis(wasmContext, {
			polarAxisMode: EPolarAxisMode.Angular,
			axisAlignment: EAxisAlignment.Top,
			polarLabelMode: EPolarLabelMode.Perpendicular,
			visibleRange: new NumberRange(0, 100),
			zoomExtentsToInitialRange: true,
			flippedCoordinates: true,
			useNativeText: true,
			totalAngleDegrees: 220,
			startAngleDegrees: -20,
			drawMinorGridLines: false,
			drawMajorGridLines: false,
			drawMinorTickLines: false,
			drawMajorTickLines: false,
			labelPrecision: 0,
		}),
	);

	sciChartSurface.annotations.add(
		new PolarArcAnnotation({
			x2: 8.1,
			x1: 10,
			y1: 0,
			y2: 100,
			fill: "#88888844",
			strokeThickness: 0,
		}),
	);

	const gaugeArcs = buildGaugeArcs(sciChartSurface, 0);

	return {
		sciChartSurface,
		controls: {
			update(needlePercent: number, confidence: number) {
				applyGaugeNeedle(gaugeArcs, needlePercent);
				gaugeArcs.label.text = formatSignalConfidence(confidence);
				sciChartSurface.invalidateElement();
			},
			dispose() {
				sciChartSurface.delete();
			},
		} satisfies SignalGaugeControls,
	};
};
