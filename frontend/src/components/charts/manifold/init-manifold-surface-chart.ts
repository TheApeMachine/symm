import {
	CameraController,
	EDrawMeshAs,
	GradientColorPalette,
	MouseWheelZoomModifier3D,
	NumberRange,
	NumericAxis3D,
	OrbitModifier3D,
	ResetCamera3DModifier,
	ScatterRenderableSeries3D,
	SciChart3DSurface,
	SpherePointMarker3D,
	SurfaceMeshRenderableSeries3D,
	UniformGridDataSeries3D,
	Vector3,
	XyzDataSeries3D,
	zeroArray2D,
} from "scichart";
import { appTheme } from "#/components/charts/fluid/theme";
import {
	manifoldCameraFrame,
	manifoldHeightExtent,
} from "#/components/charts/manifold/manifold-camera";
import {
	isDegenerateHeightmap,
	projectManifoldHeightmap,
} from "#/components/charts/manifold/manifold-grid";
import type {
	ManifoldCarrierRow,
	ManifoldFieldSnapshot,
} from "#/components/charts/manifold/types";
import { ensureSciChartWasm } from "#/lib/utils";

const MANIFOLD_GRID_X = 32;
const MANIFOLD_GRID_Z = 16;
const MANIFOLD_AXIS_LABEL_SIZE = 9;
const MANIFOLD_AXIS_TITLE_SIZE = 10;
const MANIFOLD_SYMBOL_MARKER_SIZE = 0.35;
const MANIFOLD_WHALE_MARKER_SIZE = 0.55;

export type ManifoldChartControls = {
	push: (frame: ManifoldFieldSnapshot) => boolean;
};

const fitManifoldCamera = (
	sciChart3DSurface: SciChart3DSurface,
	wasmContext: Awaited<
		ReturnType<typeof SciChart3DSurface.create>
	>["wasmContext"],
	gridX: number,
	gridZ: number,
	yMin: number,
	yMax: number,
) => {
	const frame = manifoldCameraFrame(gridX, gridZ, yMax - yMin);

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(
			frame.centerX - frame.orbit * 0.82,
			frame.orbit * 0.62,
			frame.centerZ + frame.orbit * 0.82,
		),
		target: new Vector3(frame.centerX, frame.yCenter, frame.centerZ),
	});
	sciChart3DSurface.worldDimensions = new Vector3(
		gridX,
		frame.worldHeight,
		gridZ,
	);
	sciChart3DSurface.xAxis.visibleRange = new NumberRange(
		0,
		Math.max(gridX - 1, 1),
	);
	sciChart3DSurface.zAxis.visibleRange = new NumberRange(
		0,
		Math.max(gridZ - 1, 1),
	);
	sciChart3DSurface.yAxis.visibleRange = new NumberRange(yMin, yMax);
};

const sampleSurfaceHeight = (
	heights: number[][],
	chartX: number,
	chartZ: number,
): number | null => {
	const gridZ = heights.length;
	const gridX = gridZ > 0 ? (heights[0]?.length ?? 0) : 0;

	if (gridX === 0 || gridZ === 0) {
		return null;
	}

	const xIndex = Math.min(Math.max(Math.round(chartX), 0), gridX - 1);
	const zIndex = Math.min(Math.max(Math.round(chartZ), 0), gridZ - 1);
	const value = heights[zIndex]?.[xIndex];

	if (typeof value !== "number" || !Number.isFinite(value)) {
		return null;
	}

	return value;
};

const updateCarrierSeries = (
	series: ScatterRenderableSeries3D,
	carriers: ManifoldCarrierRow[],
	grid: ManifoldFieldSnapshot["grid"],
	heights: number[][],
) => {
	const dataSeries = series.dataSeries as XyzDataSeries3D;
	const spacing = grid.spacing;

	dataSeries.clear();

	for (const carrier of carriers) {
		const chartX = carrier.x / spacing;
		const chartZ = carrier.z / spacing;
		const chartY = sampleSurfaceHeight(heights, chartX, chartZ);

		if (chartY === null) {
			continue;
		}

		dataSeries.append(chartX, chartY, chartZ);
	}
};

export const initManifoldSurfaceChart = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
			freezeWhenOutOfView: true,
		},
	);

	const axisLabelStyle = { fontSize: MANIFOLD_AXIS_LABEL_SIZE };
	const axisTitleStyle = { fontSize: MANIFOLD_AXIS_TITLE_SIZE };

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Depth (price ticks)",
		labelStyle: axisLabelStyle,
		axisTitleStyle: axisTitleStyle,
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Density ρ",
		labelStyle: axisLabelStyle,
		axisTitleStyle: axisTitleStyle,
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Cross-asset rank",
		labelStyle: axisLabelStyle,
		axisTitleStyle: axisTitleStyle,
	});

	let gridX = MANIFOLD_GRID_X;
	let gridZ = MANIFOLD_GRID_Z;
	let surfaceYMin = 0;
	let surfaceYMax = 1;
	let cameraFitted = false;
	let heightmapArray = zeroArray2D([gridZ, gridX]);

	let dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: heightmapArray,
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Manifold density projection",
		containsNaN: false,
	});

	const colorMap = new GradientColorPalette(wasmContext, {
		gradientStops: [
			{ offset: 1, color: appTheme.VividPink },
			{ offset: 0.75, color: appTheme.VividOrange },
			{ offset: 0.5, color: appTheme.VividGreen },
			{ offset: 0.25, color: appTheme.VividSkyBlue },
			{ offset: 0, color: appTheme.DarkIndigo },
		],
	});

	const series = new SurfaceMeshRenderableSeries3D(wasmContext, {
		dataSeries,
		minimum: surfaceYMin,
		maximum: surfaceYMax,
		opacity: 0.92,
		cellHardnessFactor: 0.85,
		shininess: 0.2,
		lightingFactor: 0.35,
		highlight: 1.0,
		stroke: appTheme.VividPurple,
		strokeThickness: 1,
		contourStroke: appTheme.VividTeal,
		contourInterval: 2,
		contourOffset: 0,
		contourStrokeThickness: 1,
		drawSkirt: true,
		drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
		meshColorPalette: colorMap,
		isVisible: true,
	});

	const symbolCarrierSeries = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: new XyzDataSeries3D(wasmContext, {
			dataSeriesName: "Symbol carriers",
			containsNaN: false,
		}),
		pointMarker: new SpherePointMarker3D(wasmContext, {
			size: MANIFOLD_SYMBOL_MARKER_SIZE,
			fill: appTheme.VividSkyBlue,
		}),
	});

	const whaleCarrierSeries = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: new XyzDataSeries3D(wasmContext, {
			dataSeriesName: "Whale carriers",
			containsNaN: false,
		}),
		pointMarker: new SpherePointMarker3D(wasmContext, {
			size: MANIFOLD_WHALE_MARKER_SIZE,
			fill: appTheme.VividPink,
		}),
	});

	sciChart3DSurface.renderableSeries.add(series);
	sciChart3DSurface.renderableSeries.add(symbolCarrierSeries);
	sciChart3DSurface.renderableSeries.add(whaleCarrierSeries);
	sciChart3DSurface.chartModifiers.add(new MouseWheelZoomModifier3D());
	sciChart3DSurface.chartModifiers.add(new OrbitModifier3D());
	sciChart3DSurface.chartModifiers.add(new ResetCamera3DModifier());

	fitManifoldCamera(
		sciChart3DSurface,
		wasmContext,
		gridX,
		gridZ,
		surfaceYMin,
		surfaceYMax,
	);

	const push = (frame: ManifoldFieldSnapshot): boolean => {
		if (!(frame.grid.spacing > 0)) {
			return false;
		}

		const projected = projectManifoldHeightmap(frame, 0, 1);
		const heights = projected.heights;

		if (isDegenerateHeightmap(heights)) {
			return false;
		}

		const extent = manifoldHeightExtent(heights);
		const pad = Math.max((extent.max - extent.min) * 0.08, 0.04);

		surfaceYMin = extent.min - pad;
		surfaceYMax = extent.max + pad;

		const nextGridZ = heights.length;
		const nextGridX = heights[0]?.length ?? 0;
		const gridChanged = nextGridZ !== gridZ || nextGridX !== gridX;

		if (gridChanged) {
			gridZ = nextGridZ;
			gridX = nextGridX;
			heightmapArray = zeroArray2D([gridZ, gridX]);
			dataSeries = new UniformGridDataSeries3D(wasmContext, {
				yValues: heightmapArray,
				xStep: 1,
				zStep: 1,
				dataSeriesName: "Manifold density projection",
				containsNaN: false,
			});
			series.dataSeries = dataSeries;
		}

		for (let zIndex = 0; zIndex < gridZ; zIndex += 1) {
			const heightRow = heights[zIndex];
			const targetRow = heightmapArray[zIndex];

			if (!heightRow || !targetRow) {
				continue;
			}

			for (let xIndex = 0; xIndex < gridX; xIndex += 1) {
				const value = heightRow[xIndex];
				targetRow[xIndex] =
					typeof value === "number" && Number.isFinite(value) ? value : 0;
			}
		}

		dataSeries.setYValues(heightmapArray);
		series.minimum = surfaceYMin;
		series.maximum = surfaceYMax;

		const symbolCarriers = frame.carriers.filter(
			(carrier) => carrier.role === "symbol",
		);
		const whaleCarriers = frame.carriers.filter(
			(carrier) => carrier.role === "whale",
		);

		updateCarrierSeries(
			symbolCarrierSeries,
			symbolCarriers,
			frame.grid,
			heights,
		);
		updateCarrierSeries(whaleCarrierSeries, whaleCarriers, frame.grid, heights);

		if (gridChanged || !cameraFitted) {
			fitManifoldCamera(
				sciChart3DSurface,
				wasmContext,
				gridX,
				gridZ,
				surfaceYMin,
				surfaceYMax,
			);
			cameraFitted = true;
		}
		sciChart3DSurface.invalidateElement();

		return true;
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		controls: { push },
	};
};
