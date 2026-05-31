import {
	CameraController,
	EDrawMeshAs,
	EMeshPaletteMode,
	EMeshResolution,
	GradientColorPalette,
	MouseWheelZoomModifier3D,
	NumericAxis3D,
	OrbitModifier3D,
	ResetCamera3DModifier,
	SciChart3DSurface,
	SurfaceMeshRenderableSeries3D,
	UniformGridDataSeries3D,
	Vector3,
	zeroArray2D,
} from "scichart";

import { appTheme } from "#/components/symm/theme";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const GRID_Z = 50;
const GRID_X = 50;
const Y_MIN = -0.3;
const Y_MAX = 0.3;

export type GenericSurfaceControls = {
	update: (heights: number[][]) => void;
	dispose: () => void;
};

const resampleHeights = (
	heights: number[][],
	targetZ: number,
	targetX: number,
): number[][] => {
	if (heights.length === 0) {
		return zeroArray2D([targetZ, targetX]);
	}

	const sourceZ = heights.length;
	const sourceX = heights[0]?.length ?? 0;

	if (sourceZ === 0 || sourceX === 0) {
		return zeroArray2D([targetZ, targetX]);
	}

	const output = zeroArray2D([targetZ, targetX]);

	for (let zIndex = 0; zIndex < targetZ; zIndex++) {
		const sourceZIndex = Math.min(
			sourceZ - 1,
			Math.floor((zIndex / targetZ) * sourceZ),
		);

		for (let xIndex = 0; xIndex < targetX; xIndex++) {
			const sourceXIndex = Math.min(
				sourceX - 1,
				Math.floor((xIndex / targetX) * sourceX),
			);

			output[zIndex][xIndex] = heights[sourceZIndex][sourceXIndex] ?? 0;
		}
	}

	return output;
};

export const drawGenericSurface = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
		},
	);

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-130, 160, 130),
		target: new Vector3(0, 0, 0),
	});
	sciChart3DSurface.worldDimensions = new Vector3(200, 80, 200);

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		labelStyle: { fontSize: 0 },
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		labelStyle: { fontSize: 0 },
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		labelStyle: { fontSize: 0 },
	});

	const heightmapArray = zeroArray2D([GRID_Z, GRID_X]);
	const dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: heightmapArray,
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Surface Mesh",
	});

	const colorMap = new GradientColorPalette(wasmContext, {
		gradientStops: [
			{ offset: 1, color: appTheme.VividPink },
			{ offset: 0.9, color: appTheme.VividOrange },
			{ offset: 0.7, color: appTheme.MutedRed },
			{ offset: 0.5, color: appTheme.VividGreen },
			{ offset: 0.3, color: appTheme.VividSkyBlue },
			{ offset: 0.15, color: appTheme.Indigo },
			{ offset: 0, color: appTheme.DarkIndigo },
		],
	});

	const series = new SurfaceMeshRenderableSeries3D(wasmContext, {
		dataSeries,
		minimum: Y_MIN,
		maximum: Y_MAX,
		opacity: 0.99,
		cellHardnessFactor: 1.0,
		shininess: 1.0,
		lightingFactor: 0.15,
		highlight: 1.0,
		stroke: appTheme.VividBlue,
		strokeThickness: 2.0,
		contourStroke: appTheme.VividBlue,
		contourInterval: 2.0,
		contourOffset: 0,
		contourStrokeThickness: 0.1,
		drawSkirt: false,
		drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
		meshPaletteMode: EMeshPaletteMode.HEIGHT_MAP_INTERPOLATED,
		meshResolution: EMeshResolution.MESH_RESOLUTION_X4,
		meshColorPalette: colorMap,
		isVisible: true,
	});

	sciChart3DSurface.renderableSeries.add(series);
	sciChart3DSurface.chartModifiers.add(new MouseWheelZoomModifier3D());
	sciChart3DSurface.chartModifiers.add(new OrbitModifier3D());
	sciChart3DSurface.chartModifiers.add(new ResetCamera3DModifier());

	const controls: GenericSurfaceControls = {
		update: (heights) => {
			const resampled = resampleHeights(heights, GRID_Z, GRID_X);

			for (let zIndex = 0; zIndex < GRID_Z; zIndex++) {
				for (let xIndex = 0; xIndex < GRID_X; xIndex++) {
					heightmapArray[zIndex][xIndex] = resampled[zIndex][xIndex];
				}
			}

			dataSeries.setYValues(heightmapArray);
			sciChart3DSurface.invalidateElement();
		},
		dispose: () => undefined,
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		controls,
	};
};
