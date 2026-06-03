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
	zeroArray2D,
} from "scichart";
import {
	buildFluidGrid,
	FLUID_GRID_SIZE,
} from "#/components/charts/fluid/fluid-grid";
import { ensureSciChartWasm } from "#/lib/utils";
import { appTheme } from "#/components/charts/fluid/theme";
import type { FluidSymbolRow } from "#/components/charts/fluid/types";

export const initFluidSurfaceChart = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
		},
	);

	// Create and position the camera in the 3D world
	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-150, 200, 150),
		target: new Vector3(0, 50, 0),
	});
	// Set the worlddimensions, which defines the Axis cube size
	sciChart3DSurface.worldDimensions = new Vector3(200, 100, 200);

	// Add an X,Y and Z Axis
	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "X Axis",
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Y Axis",
		visibleRange: new NumberRange(-0.3, 0.3),
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Z Axis",
	});

	const gridSize = FLUID_GRID_SIZE;
	const heightmapArray = zeroArray2D([gridSize, gridSize]);

	// Create a UniformGridDataSeries3D
	const dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: heightmapArray,
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Uniform Surface Mesh",
	});

	// Create the color map
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

	// Finally, create a SurfaceMeshRenderableSeries3D and add to the chart
	const series = new SurfaceMeshRenderableSeries3D(wasmContext, {
		dataSeries,
		minimum: -0.3,
		maximum: 0.5,
		opacity: 0.9,
		cellHardnessFactor: 1.0,
		shininess: 0,
		lightingFactor: 0.0,
		highlight: 1.0,
		stroke: appTheme.VividBlue,
		strokeThickness: 2.0,
		contourStroke: appTheme.VividBlue,
		contourInterval: 2,
		contourOffset: 0,
		contourStrokeThickness: 2,
		drawSkirt: false,
		drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
		meshColorPalette: colorMap,
		isVisible: true,
	});

	sciChart3DSurface.renderableSeries.add(series);

	// Optional: Add some interactivity modifiers
	sciChart3DSurface.chartModifiers.add(new MouseWheelZoomModifier3D());
	sciChart3DSurface.chartModifiers.add(new OrbitModifier3D());
	sciChart3DSurface.chartModifiers.add(new ResetCamera3DModifier());

	const push = (raw: unknown) => {
		if (typeof raw !== "object" || raw === null) {
			return;
		}

		const row = raw as Record<string, unknown>;

		if (row.type !== "fluid" || !Array.isArray(row.symbols)) {
			return;
		}

		const symbols = row.symbols as FluidSymbolRow[];

		if (symbols.length === 0) {
			return;
		}

		const grid = buildFluidGrid(symbols);
		const heights = grid.heights;

		for (let zIndex = 0; zIndex < gridSize; zIndex++) {
			const heightRow = heights[zIndex];

			if (!heightRow) {
				continue;
			}

			for (let xIndex = 0; xIndex < gridSize; xIndex++) {
				const value = heightRow[xIndex];
				heightmapArray[zIndex][xIndex] =
					typeof value === "number" && Number.isFinite(value) ? value : 0;
			}
		}

		dataSeries.setYValues(heightmapArray);
		series.minimum = grid.min;
		series.maximum = grid.max;
		sciChart3DSurface.yAxis.visibleRange = new NumberRange(grid.min, grid.max);
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		controls: { push },
	};
};
