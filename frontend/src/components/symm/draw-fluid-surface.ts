import {
	CameraController,
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
	type TSciChart3D,
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

export type FluidSurfaceControls = {
	update: (snapshot: FieldSnapshotEvent) => void;
	dispose: () => void;
};

export const drawFluidSurface = async (rootElement: HTMLDivElement) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			disableAspect: false,
			background: PALETTE.dark,
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

	const gridRange = new NumberRange(0, FLUID_GRID_SIZE - 1);
	const half = FLUID_GRID_SIZE / 2;
	const yMid = FLUID_DISPLAY_HEIGHT / 2;
	const orbit = FLUID_GRID_SIZE * 1.2;

	sciChart3DSurface.worldDimensions = new Vector3(
		FLUID_GRID_SIZE,
		FLUID_DISPLAY_HEIGHT,
		FLUID_GRID_SIZE,
	);
	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(
			half - orbit * 0.82,
			yMid + orbit * 0.62,
			half + orbit * 0.82,
		),
		target: new Vector3(half, yMid, half),
	});

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		...unlabeledAxis,
		visibleRange: gridRange,
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, unlabeledAxis);
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		...unlabeledAxis,
		visibleRange: gridRange,
	});

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
		maximum: 1,
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
	sciChart3DSurface.chartModifiers.add(
		new MouseWheelZoomModifier3D(),
		new OrbitModifier3D(),
		new ResetCamera3DModifier(),
	);

	let followView = true;
	const interactionAbort = new AbortController();

	const applyViewFrame = () => {
		const yAxis = sciChart3DSurface.yAxis;

		if (yAxis) {
			yAxis.visibleRange = new NumberRange(0, FLUID_DISPLAY_HEIGHT);
		}

		sciChart3DSurface.worldDimensions = new Vector3(
			FLUID_GRID_SIZE,
			FLUID_DISPLAY_HEIGHT,
			FLUID_GRID_SIZE,
		);
		sciChart3DSurface.camera.target = new Vector3(half, yMid, half);
	};

	const applyGrid = (grid: FluidGrid) => {
		const size = Math.min(grid.heights.length, FLUID_GRID_SIZE);
		const scaleMax = Math.max(grid.outliers.displayMax, grid.max, 0.05);
		const floor = scaleMax * 0.02;

		for (let z = 0; z < FLUID_GRID_SIZE; z++) {
			for (let x = 0; x < FLUID_GRID_SIZE; x++) {
				heightmap[z][x] = 0;
			}
		}

		for (let z = 0; z < size; z++) {
			const row = grid.heights[z];

			if (!row) {
				continue;
			}

			const width = Math.min(row.length, FLUID_GRID_SIZE);

			for (let x = 0; x < width; x++) {
				const raw = row[x];

				if (!Number.isFinite(raw) || raw <= floor) {
					continue;
				}

				heightmap[z][x] = Math.min(
					FLUID_DISPLAY_HEIGHT,
					(raw / scaleMax) * FLUID_DISPLAY_HEIGHT,
				);
			}
		}

		dataSeries.setYValues(heightmap);
		meshSeries.minimum = 0;
		meshSeries.maximum = FLUID_DISPLAY_HEIGHT;

		if (followView) {
			applyViewFrame();
		}

		sciChart3DSurface.invalidateElement();
	};

	const leaveFollowMode = () => {
		followView = false;
	};

	rootElement.addEventListener("wheel", leaveFollowMode, {
		passive: true,
		signal: interactionAbort.signal,
	});
	rootElement.addEventListener("pointerdown", leaveFollowMode, {
		passive: true,
		signal: interactionAbort.signal,
	});
	rootElement.addEventListener(
		"dblclick",
		() => {
			followView = true;
			applyViewFrame();
			sciChart3DSurface.invalidateElement();
		},
		{ passive: true, signal: interactionAbort.signal },
	);

	requestAnimationFrame(() => {
		const width = Math.floor(rootElement.clientWidth);
		const height = Math.floor(rootElement.clientHeight);

		if (width > 0 && height > 0) {
			sciChart3DSurface.onResize(width, height);
			sciChart3DSurface.invalidateElement();
		}
	});

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
