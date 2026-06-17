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
	latentCameraFrame,
	latentWorldCameraFrame,
} from "#/components/charts/resonance/latent-camera";
import {
	type LatentPoint3D,
	latentMarkerSizes,
	latentPoint3D,
	parseResonanceXRayFrame,
} from "#/components/charts/resonance/resonance-xray-frame";
import { appTheme } from "#/components/charts/shared/theme";
import { ensureSciChartWasm } from "#/lib/utils";

const LATENT_TRAIL_CAPACITY = 512;
const MIN_ORBIT_RADIUS = 0.75;

const surpriseFill = (surprise: number, surpriseScale: number): string => {
	const normalized = Math.min(1, Math.max(0, surprise / surpriseScale));

	if (normalized < 0.5) {
		return appTheme.VividGreen;
	}

	if (normalized < 0.8) {
		return appTheme.VividOrange;
	}

	return appTheme.VividRed;
};

const readTrailPoints = (trailSeries: XyzDataSeries3D): LatentPoint3D[] => {
	const count = trailSeries.count();

	if (count === 0) {
		return [];
	}

	const xValues = trailSeries.getNativeXValues();
	const yValues = trailSeries.getNativeYValues();
	const zValues = trailSeries.getNativeZValues();
	const points: LatentPoint3D[] = [];

	for (let pointIndex = 0; pointIndex < count; pointIndex += 1) {
		points.push({
			x: xValues.get(pointIndex),
			y: yValues.get(pointIndex),
			z: zValues.get(pointIndex),
		});
	}

	return points;
};

const fitLatentCamera = (
	sciChart3DSurface: SciChart3DSurface,
	wasmContext: Awaited<
		ReturnType<typeof SciChart3DSurface.create>
	>["wasmContext"],
	trailSeries: XyzDataSeries3D,
	headMarker: SpherePointMarker3D,
	trailMarker: SpherePointMarker3D,
) => {
	const frame = latentCameraFrame(
		readTrailPoints(trailSeries),
		MIN_ORBIT_RADIUS,
	);

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

	headMarker.size = worldFrame.markerSizes.head;
	trailMarker.size = worldFrame.markerSizes.trail;
};

export const initResonanceLatentChart = async (
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

	const trailSeries = new XyzDataSeries3D(wasmContext, {
		dataSeriesName: "Latent trajectory",
		fifoCapacity: LATENT_TRAIL_CAPACITY,
		capacity: LATENT_TRAIL_CAPACITY,
	});

	const headSeries = new XyzDataSeries3D(wasmContext, {
		dataSeriesName: "Latent head",
	});

	const initialMarkerSizes = latentMarkerSizes(MIN_ORBIT_RADIUS);

	const trailMarker = new SpherePointMarker3D(wasmContext, {
		size: initialMarkerSizes.trail,
		fill: `${appTheme.VividSkyBlue}AA`,
	});

	const headMarker = new SpherePointMarker3D(wasmContext, {
		size: initialMarkerSizes.head,
		fill: appTheme.VividPink,
	});

	const trailRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: trailSeries,
		pointMarker: trailMarker,
		isVisible: false,
	});

	const headRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: headSeries,
		pointMarker: headMarker,
		isVisible: false,
	});

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(-1.1, 0.85, 1.1),
		target: new Vector3(1, 1, 1),
	});
	sciChart3DSurface.worldDimensions = new Vector3(2, 2, 2);

	sciChart3DSurface.renderableSeries.add(trailRenderable, headRenderable);

	let surpriseScale = 1;
	let activeSymbol = "";
	let hasRenderablePoints = false;

	const setHeadPoint = (point: LatentPoint3D) => {
		if (headSeries.count() === 0) {
			headSeries.append(point.x, point.y, point.z);

			return;
		}

		headSeries.getNativeXValues().set(0, point.x);
		headSeries.getNativeYValues().set(0, point.y);
		headSeries.getNativeZValues().set(0, point.z);
	};

	const addData = (raw: Record<string, unknown>) => {
		if (sciChart3DSurface.isDeleted) {
			return;
		}

		const frame = parseResonanceXRayFrame(raw);

		if (frame === null) {
			return;
		}

		if (activeSymbol !== "" && activeSymbol !== frame.symbol) {
			trailSeries.clear();
		}

		activeSymbol = frame.symbol;

		const point = latentPoint3D(frame.layers);

		trailSeries.append(point.x, point.y, point.z);
		setHeadPoint(point);

		surpriseScale = Math.max(surpriseScale * 0.95, frame.surprise, 1e-6);
		headMarker.fill = surpriseFill(frame.surprise, surpriseScale);

		if (!hasRenderablePoints) {
			hasRenderablePoints = true;
			trailRenderable.isVisible = true;
			headRenderable.isVisible = true;
		}

		fitLatentCamera(
			sciChart3DSurface,
			wasmContext,
			trailSeries,
			headMarker,
			trailMarker,
		);
		sciChart3DSurface.invalidateElement();
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		addData,
	};
};
