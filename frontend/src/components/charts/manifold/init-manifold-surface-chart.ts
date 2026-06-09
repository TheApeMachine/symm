import {
	CameraController,
	EDrawMeshAs,
	GradientColorPalette,
	MouseWheelZoomModifier3D,
	NumberRange,
	NumericAxis3D,
	OrbitModifier3D,
	ResetCamera3DModifier,
	SciChart3DSurface,
	SurfaceMeshRenderableSeries3D,
	UniformGridDataSeries3D,
	Vector3,
} from "scichart";
import { appTheme } from "#/components/charts/fluid/theme";
import { projectManifoldHeightmap } from "#/components/charts/manifold/manifold-grid";
import type { ManifoldFieldSnapshot } from "#/components/charts/manifold/types";
import { ensureSciChartWasm } from "#/lib/utils";

const MANIFOLD_SURFACE_Y_MIN = 0;
const MANIFOLD_SURFACE_Y_MAX = 1;

export type ManifoldChartControls = {
	push: (frame: ManifoldFieldSnapshot) => void;
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

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-180, 220, 180),
		target: new Vector3(16, 0.35, 8),
	});
	sciChart3DSurface.worldDimensions = new Vector3(64, 20, 32);

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Depth (price ticks)",
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Density ρ",
		visibleRange: new NumberRange(
			MANIFOLD_SURFACE_Y_MIN,
			MANIFOLD_SURFACE_Y_MAX,
		),
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Cross-asset rank",
	});

	let dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: [[0]],
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Manifold density projection",
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
		minimum: MANIFOLD_SURFACE_Y_MIN,
		maximum: MANIFOLD_SURFACE_Y_MAX,
		opacity: 0.92,
		cellHardnessFactor: 0.85,
		shininess: 0.2,
		lightingFactor: 0.35,
		highlight: 1.0,
		stroke: appTheme.VividPurple,
		strokeThickness: 1.5,
		contourStroke: appTheme.VividTeal,
		contourInterval: 2,
		contourOffset: 0,
		contourStrokeThickness: 1.5,
		drawSkirt: true,
		drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
		meshColorPalette: colorMap,
		isVisible: true,
	});

	sciChart3DSurface.renderableSeries.add(series);
	sciChart3DSurface.chartModifiers.add(new MouseWheelZoomModifier3D());
	sciChart3DSurface.chartModifiers.add(new OrbitModifier3D());
	sciChart3DSurface.chartModifiers.add(new ResetCamera3DModifier());

	const push = (frame: ManifoldFieldSnapshot) => {
		const projected = projectManifoldHeightmap(
			frame,
			MANIFOLD_SURFACE_Y_MIN,
			MANIFOLD_SURFACE_Y_MAX,
		);
		const heights = projected.heights;

		if (heights.length === 0 || (heights[0]?.length ?? 0) === 0) {
			return;
		}

		dataSeries = new UniformGridDataSeries3D(wasmContext, {
			yValues: heights,
			xStep: 1,
			zStep: 1,
			dataSeriesName: "Manifold density projection",
		});
		series.dataSeries = dataSeries;

		sciChart3DSurface.worldDimensions = new Vector3(
			Math.max(projected.gridX, 1),
			20,
			Math.max(projected.gridZ, 1),
		);
		sciChart3DSurface.yAxis.visibleRange = new NumberRange(
			MANIFOLD_SURFACE_Y_MIN,
			MANIFOLD_SURFACE_Y_MAX,
		);
		sciChart3DSurface.invalidateElement();
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		controls: { push },
	};
};
