import {
	EDrawMeshAs,
	EAxisPlaneDrawLabelsMode,
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
import {
	FLUID_GRID_SIZE,
	gridFromSnapshot,
	type FluidGrid,
	gridFromPayload,
} from "#/lib/symm/fluid-grid";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const PALETTE = {
	dark: "#0f172a",
	indigo: "#6366f1",
	sky: "#38bdf8",
	green: "#4ade80",
	orange: "#fb923c",
	pink: "#f472b6",
	stroke: "#38bdf8",
};

const FLUID_DISPLAY_HEIGHT = FLUID_GRID_SIZE * 0.4;

/** SciChart axis cube — larger than the data grid so the mesh sits on the origin plane like the official demo. */
const WORLD_X = 100;
const WORLD_Y = 50;
const WORLD_Z = 100;

/** Horizontal orbit offset from the SciChart demo bearing (degrees). */
const CAMERA_YAW_OFFSET_DEG = 90;

const defaultCameraPosition = () =>
	new Vector3(-WORLD_X, WORLD_Y * 1.5, WORLD_Z);

const defaultCameraTarget = () => new Vector3(0, WORLD_Y / 2, 0);

export type FluidSurfaceControls = {
	update: (snapshot: FieldSnapshotEvent) => void;
	dispose: () => void;
};

export const drawFluidSurface = async (rootElement: HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			background: PALETTE.dark,
			worldDimensions: new Vector3(WORLD_X, WORLD_Y, WORLD_Z),
			cameraOptions: {
				position: defaultCameraPosition(),
				target: defaultCameraTarget(),
			},
			xyAxisPlane: {
				drawLabelsMode: EAxisPlaneDrawLabelsMode.Hidden,
				drawTitlesMode: EAxisPlaneDrawLabelsMode.Hidden,
			},
			zyAxisPlane: {
				drawLabelsMode: EAxisPlaneDrawLabelsMode.Hidden,
				drawTitlesMode: EAxisPlaneDrawLabelsMode.Hidden,
			},
			zxAxisPlane: {
				drawLabelsMode: EAxisPlaneDrawLabelsMode.Hidden,
				drawTitlesMode: EAxisPlaneDrawLabelsMode.Hidden,
			},
		},
	);

	const unlabeledAxis = {
		drawLabels: false,
		drawMajorTickLines: false,
		drawMinorTickLines: false,
	} as const;

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, unlabeledAxis);
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		...unlabeledAxis,
		visibleRange: new NumberRange(0, FLUID_DISPLAY_HEIGHT),
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, unlabeledAxis);

	const heightmap = zeroArray2D([FLUID_GRID_SIZE, FLUID_GRID_SIZE]);
	const dataSeries = new UniformGridDataSeries3D(wasmContext, {
		yValues: heightmap,
		xStep: 1,
		zStep: 1,
		dataSeriesName: "Market fluid",
	});

	const colorMap = new GradientColorPalette(wasmContext, {
		gradientStops: [
			{ offset: 1, color: PALETTE.pink },
			{ offset: 0.75, color: PALETTE.orange },
			{ offset: 0.5, color: PALETTE.green },
			{ offset: 0.3, color: PALETTE.sky },
			{ offset: 0.1, color: PALETTE.indigo },
			{ offset: 0, color: PALETTE.dark },
		],
	});

	const meshSeries = new SurfaceMeshRenderableSeries3D(wasmContext, {
		dataSeries,
		minimum: 0,
		maximum: FLUID_DISPLAY_HEIGHT,
		opacity: 0.92,
		cellHardnessFactor: 0.85,
		shininess: 0.1,
		lightingFactor: 0.35,
		highlight: 0.8,
		stroke: PALETTE.stroke,
		strokeThickness: 0.5,
		drawSkirt: false,
		drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
		meshColorPalette: colorMap,
	});

	sciChart3DSurface.renderableSeries.add(meshSeries);

	const resetCamera = () => {
		const camera = sciChart3DSurface.camera;
		camera.position = defaultCameraPosition();
		camera.target = defaultCameraTarget();
		camera.orbitalYaw += CAMERA_YAW_OFFSET_DEG;
	};

	resetCamera();

	sciChart3DSurface.chartModifiers.add(
		new MouseWheelZoomModifier3D(),
		new OrbitModifier3D(),
		new ResetCamera3DModifier(),
	);

	const applyGrid = (grid: FluidGrid) => {
		const size = Math.min(grid.heights.length, FLUID_GRID_SIZE);
		const scaleMax = Math.max(grid.outliers.displayMax, grid.max, 0.05);
		const floor = scaleMax * 0.02;

		for (let zIndex = 0; zIndex < FLUID_GRID_SIZE; zIndex++) {
			for (let xIndex = 0; xIndex < FLUID_GRID_SIZE; xIndex++) {
				heightmap[zIndex][xIndex] = 0;
			}
		}

		for (let zIndex = 0; zIndex < size; zIndex++) {
			const row = grid.heights[zIndex];

			if (!row) {
				continue;
			}

			const width = Math.min(row.length, FLUID_GRID_SIZE);

			for (let xIndex = 0; xIndex < width; xIndex++) {
				const raw = row[xIndex];

				if (!Number.isFinite(raw) || raw <= floor) {
					continue;
				}

				heightmap[zIndex][xIndex] = Math.min(
					FLUID_DISPLAY_HEIGHT,
					(raw / scaleMax) * FLUID_DISPLAY_HEIGHT,
				);
			}
		}

		dataSeries.setYValues(heightmap);
		sciChart3DSurface.invalidateElement();
	};

	const interactionAbort = new AbortController();

	rootElement.addEventListener(
		"dblclick",
		() => {
			resetCamera();
			sciChart3DSurface.invalidateElement();
		},
		{ passive: true, signal: interactionAbort.signal },
	);

	const controls: FluidSurfaceControls = {
		update: (snapshot: FieldSnapshotEvent) => {
			const grid = snapshot.grid?.heights?.length
				? gridFromPayload(snapshot.grid)
				: gridFromSnapshot(snapshot);
			applyGrid(grid);
		},
		dispose: () => {
			interactionAbort.abort();
		},
	};

	return { sciChartSurface: sciChart3DSurface, wasmContext, controls };
};
