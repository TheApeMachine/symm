import {
	CameraController,
	NumberRange,
	NumericAxis3D,
	ScatterRenderableSeries3D,
	SciChart3DSurface,
	SpherePointMarker3D,
	Vector3,
	XyzDataSeries3D,
} from "scichart";
import {
	constellationCameraFrame,
	latentWorldCameraFrame,
} from "#/components/charts/resonance/latent-camera";
import {
	categoryFill,
	latentPointFromVector,
	parseResonanceUniverseFrame,
} from "#/components/charts/resonance/resonance-universe-frame";
import {
	type LatentPoint3D,
	latentMarkerSizes,
} from "#/components/charts/resonance/resonance-xray-frame";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const MIN_ORBIT_RADIUS = 0.75;

const fitConstellationCamera = (
	sciChart3DSurface: SciChart3DSurface,
	wasmContext: Awaited<
		ReturnType<typeof SciChart3DSurface.create>
	>["wasmContext"],
	points: LatentPoint3D[],
	fieldMarker: SpherePointMarker3D,
	focusMarker: SpherePointMarker3D,
) => {
	const frame = constellationCameraFrame(points, MIN_ORBIT_RADIUS);

	if (frame === null) {
		return;
	}

	const worldFrame = latentWorldCameraFrame(frame);

	sciChart3DSurface.xAxis.visibleRange = new NumberRange(
		worldFrame.visibleRange.x.min,
		worldFrame.visibleRange.x.max,
	);
	sciChart3DSurface.yAxis.visibleRange = new NumberRange(
		worldFrame.visibleRange.y.min,
		worldFrame.visibleRange.y.max,
	);
	sciChart3DSurface.zAxis.visibleRange = new NumberRange(
		worldFrame.visibleRange.z.min,
		worldFrame.visibleRange.z.max,
	);

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(
			worldFrame.cameraPosition.x,
			worldFrame.cameraPosition.y,
			worldFrame.cameraPosition.z,
		),
		target: new Vector3(
			worldFrame.worldCenter.x,
			worldFrame.worldCenter.y,
			worldFrame.worldCenter.z,
		),
	});
	sciChart3DSurface.worldDimensions = new Vector3(
		worldFrame.worldDimensions.x,
		worldFrame.worldDimensions.y,
		worldFrame.worldDimensions.z,
	);

	fieldMarker.size = worldFrame.markerSizes.trail;
	focusMarker.size = worldFrame.markerSizes.head;
};

export const initResonanceConstellationChart = async (
	rootElement: string | HTMLDivElement,
) => {
	await ensureSciChartWasm();

	const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(
		rootElement,
		{
			theme: appTheme.SciChartJsTheme,
			freezeWhenOutOfView: false,
		},
	);

	sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "z0",
		axisTitleStyle: { fontSize: 10, color: appTheme.ForegroundColor },
	});
	sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "z1",
		axisTitleStyle: { fontSize: 10, color: appTheme.ForegroundColor },
	});
	sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
		axisTitle: "z2",
		axisTitleStyle: { fontSize: 10, color: appTheme.ForegroundColor },
	});

	const fieldSeries = new XyzDataSeries3D(wasmContext, {
		dataSeriesName: "Resonance field",
	});
	const focusSeries = new XyzDataSeries3D(wasmContext, {
		dataSeriesName: "Resonance focus",
	});

	const initialMarkerSizes = latentMarkerSizes(MIN_ORBIT_RADIUS);

	const fieldMarker = new SpherePointMarker3D(wasmContext, {
		size: initialMarkerSizes.trail,
		fill: `${appTheme.VividSkyBlue}AA`,
	});
	const focusMarker = new SpherePointMarker3D(wasmContext, {
		size: initialMarkerSizes.head,
		fill: appTheme.VividPink,
	});

	const fieldRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: fieldSeries,
		pointMarker: fieldMarker,
		isVisible: false,
	});
	const focusRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: focusSeries,
		pointMarker: focusMarker,
		isVisible: false,
	});

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-1.1, 0.85, 1.1),
		target: new Vector3(1, 1, 1),
	});
	sciChart3DSurface.worldDimensions = new Vector3(2, 2, 2);

	sciChart3DSurface.renderableSeries.add(fieldRenderable, focusRenderable);

	let hasRenderablePoints = false;

	const addData = (raw: Record<string, unknown>) => {
		if (sciChart3DSurface.isDeleted) {
			return;
		}

		const frame = parseResonanceUniverseFrame(raw);

		if (frame === null) {
			return;
		}

		const points: LatentPoint3D[] = [];

		for (const summary of frame.symbols) {
			const point = latentPointFromVector(summary.latent);

			if (point === null) {
				return;
			}

			points.push(point);
		}

		if (points.length === 0) {
			return;
		}

		const focusSummary = frame.symbols.find(
			(summary) => summary.symbol === frame.focusSymbol,
		);

		if (focusSummary === undefined) {
			return;
		}

		const focusPoint = latentPointFromVector(focusSummary.latent);

		if (focusPoint === null) {
			return;
		}

		fieldSeries.clear();
		focusSeries.clear();

		for (const point of points) {
			fieldSeries.append(point.x, point.y, point.z);
		}

		focusMarker.fill = categoryFill(frame.focus.category);
		focusSeries.append(focusPoint.x, focusPoint.y, focusPoint.z);

		if (!hasRenderablePoints) {
			hasRenderablePoints = true;
			fieldRenderable.isVisible = true;
			focusRenderable.isVisible = true;
		}

		fitConstellationCamera(
			sciChart3DSurface,
			wasmContext,
			points,
			fieldMarker,
			focusMarker,
		);
		sciChart3DSurface.invalidateElement();
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		addData,
	};
};
