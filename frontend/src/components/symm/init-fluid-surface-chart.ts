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

import type { FieldSnapshotEvent } from "#/lib/symm/events";
import { appTheme } from "#/components/symm/theme";
import {
	type FluidGrid,
	gridFromPayload,
	gridFromSnapshot,
	projectFluidGridToHeightmap,
} from "#/lib/symm/fluid-grid";
import {
	defaultFluidVisualParams,
	type FluidVisualParams,
} from "#/lib/symm/fluid-visual-params";
import { ensureSciChartWasm } from "#/lib/symm/scichart-setup";

const GRID_Z = 50;
const GRID_X = 50;

export type FluidSurfaceControls = {
	update: (snapshot: FieldSnapshotEvent) => void;
	applyVisualParams: (params: FluidVisualParams) => void;
	getVisualParams: () => FluidVisualParams;
	dispose: () => void;
};

export const createDrawExample =
	(initialParams: FluidVisualParams = defaultFluidVisualParams()) =>
	async (rootElement: string | HTMLDivElement) => {
		await ensureSciChartWasm();

		let visualParams = { ...initialParams };
		let latestGrid: FluidGrid | undefined;

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
			minimum: visualParams.yMin,
			maximum: visualParams.yMax,
			opacity: visualParams.opacity,
			cellHardnessFactor: visualParams.cellHardnessFactor,
			shininess: visualParams.shininess,
			lightingFactor: visualParams.lightingFactor,
			highlight: visualParams.highlight,
			stroke: appTheme.VividBlue,
			strokeThickness: visualParams.strokeThickness,
			contourStroke: appTheme.VividBlue,
			contourInterval: visualParams.contourInterval,
			contourOffset: 0,
			contourStrokeThickness: visualParams.contourStrokeThickness,
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

		const applyLiveGrid = (grid: FluidGrid) => {
			latestGrid = grid;

			const projected = projectFluidGridToHeightmap(
				grid,
				GRID_Z,
				GRID_X,
				visualParams.yMin,
				visualParams.yMax,
			);

			for (let zIndex = 0; zIndex < GRID_Z; zIndex++) {
				for (let xIndex = 0; xIndex < GRID_X; xIndex++) {
					heightmapArray[zIndex][xIndex] = projected.display[zIndex][xIndex];
				}
			}

			dataSeries.setYValues(heightmapArray);
		};

		const applySeriesVisuals = () => {
			series.minimum = visualParams.yMin;
			series.maximum = visualParams.yMax;
			series.opacity = visualParams.opacity;
			series.cellHardnessFactor = visualParams.cellHardnessFactor;
			series.shininess = visualParams.shininess;
			series.lightingFactor = visualParams.lightingFactor;
			series.highlight = visualParams.highlight;
			series.strokeThickness = visualParams.strokeThickness;
			series.contourInterval = visualParams.contourInterval;
			series.contourStrokeThickness = visualParams.contourStrokeThickness;
		};

		const fitCamera = () => {
			const host =
				typeof rootElement === "string"
					? document.getElementById(rootElement)
					: rootElement;

			if (host === null) {
				return;
			}

			const width = host.clientWidth;
			const height = host.clientHeight;

			if (width <= 0 || height <= 0) {
				return;
			}

			const span = Math.max(width / 4, height / 4);
			const distance = span * visualParams.cameraDistanceFactor;
			const lift =
				Math.min(height / 2, width / 2) * visualParams.cameraLiftFactor;

			sciChart3DSurface.camera.position = new Vector3(
				-distance,
				lift,
				distance,
			);
			sciChart3DSurface.camera.target = new Vector3(0, 0, 0);
		};

		let resizeObserver: ResizeObserver | undefined;

		if (
			typeof ResizeObserver !== "undefined" &&
			typeof rootElement !== "string"
		) {
			resizeObserver = new ResizeObserver(() => {
				fitCamera();
			});
			resizeObserver.observe(rootElement);
		}

		const applyVisualParams = (params: FluidVisualParams) => {
			visualParams = { ...params };
			applySeriesVisuals();
			fitCamera();

			if (latestGrid !== undefined) {
				applyLiveGrid(latestGrid);
			}

			sciChart3DSurface.invalidateElement();
		};

		fitCamera();
		applySeriesVisuals();

		const controls: FluidSurfaceControls = {
			update: (snapshot: FieldSnapshotEvent) => {
				const grid = snapshot.grid?.heights?.length
					? gridFromPayload(snapshot.grid)
					: gridFromSnapshot(snapshot);

				applyLiveGrid(grid);
				sciChart3DSurface.invalidateElement();
			},
			applyVisualParams,
			getVisualParams: () => ({ ...visualParams }),
			dispose: () => {
				resizeObserver?.disconnect();
			},
		};

		return {
			sciChartSurface: sciChart3DSurface,
			wasmContext,
			controls,
		};
	};

export const drawExample = createDrawExample();
