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

import type { FieldSnapshotEvent } from "#/lib/symm/events";
import { appTheme } from "#/components/symm/theme";
import {
	type FluidGrid,
	gridFromPayload,
	gridFromSnapshot,
} from "#/lib/symm/fluid-grid";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const GRID_Z = 50;
const GRID_X = 50;
const Y_MIN = -0.3;
const Y_MAX = 0.5;

export type FluidSurfaceControls = {
	update: (snapshot: FieldSnapshotEvent) => void;
	dispose: () => void;
};

const applyLiveGrid = (grid: FluidGrid, heightmapArray: number[][]) => {
	const srcSize = grid.heights.length;
	const span = grid.max - grid.min;
	const useSpan = Number.isFinite(span) && span > 1e-6;
	const scaleMax = useSpan
		? span
		: Math.max(grid.outliers.displayMax, grid.max, 0.05);
	const base = useSpan ? grid.min : 0;
	const ySpan = Y_MAX - Y_MIN;

	for (let zIndex = 0; zIndex < GRID_Z; zIndex++) {
		for (let xIndex = 0; xIndex < GRID_X; xIndex++) {
			heightmapArray[zIndex][xIndex] = 0;
		}
	}

	if (srcSize === 0) {
		return;
	}

	for (let zIndex = 0; zIndex < GRID_Z; zIndex++) {
		const srcZ = Math.min(srcSize - 1, Math.floor((zIndex * srcSize) / GRID_Z));
		const row = grid.heights[srcZ];

		if (!row) {
			continue;
		}

		const rowLen = row.length;

		for (let xIndex = 0; xIndex < GRID_X; xIndex++) {
			const srcX = Math.min(rowLen - 1, Math.floor((xIndex * rowLen) / GRID_X));
			const raw = row[srcX];

			if (!Number.isFinite(raw) || raw <= 0) {
				continue;
			}

			const normalized = useSpan ? (raw - base) / scaleMax : raw / scaleMax;

			heightmapArray[zIndex][xIndex] = Y_MIN + normalized * ySpan;
		}
	}
};

export const drawExample = async (rootElement: string | HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
		},
	);

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-150, 200, 150),
		target: new Vector3(0, 50, 0),
	});
	sciChart3DSurface.worldDimensions = new Vector3(200, 100, 200);

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "X Axis",
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Y Axis",
		visibleRange: new NumberRange(Y_MIN, Y_MAX),
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "Z Axis",
	});

	const heightmapArray = zeroArray2D([GRID_Z, GRID_X]);

	const dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: heightmapArray,
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Uniform Surface Mesh",
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

	sciChart3DSurface.chartModifiers.add(new MouseWheelZoomModifier3D());
	sciChart3DSurface.chartModifiers.add(new OrbitModifier3D());
	sciChart3DSurface.chartModifiers.add(new ResetCamera3DModifier());

	const controls: FluidSurfaceControls = {
		update: (snapshot: FieldSnapshotEvent) => {
			const grid = snapshot.grid?.heights?.length
				? gridFromPayload(snapshot.grid)
				: gridFromSnapshot(snapshot);

			applyLiveGrid(grid, heightmapArray);
			dataSeries.setYValues(heightmapArray);
			sciChart3DSurface.invalidateElement();
		},
		dispose: () => {},
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		controls,
	};
};
