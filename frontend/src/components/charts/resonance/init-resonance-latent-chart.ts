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

const fitLatentCamera = (
	sciChart3DSurface: SciChart3DSurface,
	wasmContext: Awaited<
		ReturnType<typeof SciChart3DSurface.create>
	>["wasmContext"],
	trailSeries: XyzDataSeries3D,
) => {
	const count = trailSeries.count();

	if (count === 0) {
		return;
	}

	const xValues = trailSeries.getNativeXValues();
	const yValues = trailSeries.getNativeYValues();
	const zValues = trailSeries.getNativeZValues();

	let minX = Number.POSITIVE_INFINITY;
	let maxX = Number.NEGATIVE_INFINITY;
	let minY = Number.POSITIVE_INFINITY;
	let maxY = Number.NEGATIVE_INFINITY;
	let minZ = Number.POSITIVE_INFINITY;
	let maxZ = Number.NEGATIVE_INFINITY;

	for (let pointIndex = 0; pointIndex < count; pointIndex += 1) {
		const xValue = xValues.get(pointIndex);
		const yValue = yValues.get(pointIndex);
		const zValue = zValues.get(pointIndex);

		minX = Math.min(minX, xValue);
		maxX = Math.max(maxX, xValue);
		minY = Math.min(minY, yValue);
		maxY = Math.max(maxY, yValue);
		minZ = Math.min(minZ, zValue);
		maxZ = Math.max(maxZ, zValue);
	}

	const spanX = maxX - minX;
	const spanY = maxY - minY;
	const spanZ = maxZ - minZ;
	const orbit = Math.max(spanX, spanY, spanZ, MIN_ORBIT_RADIUS) * 1.35;
	const centerX = (minX + maxX) * 0.5;
	const centerY = (minY + maxY) * 0.5;
	const centerZ = (minZ + maxZ) * 0.5;

	sciChart3DSurface.xAxis.visibleRange = new NumberRange(
		centerX - orbit,
		centerX + orbit,
	);
	sciChart3DSurface.yAxis.visibleRange = new NumberRange(
		centerY - orbit,
		centerY + orbit,
	);
	sciChart3DSurface.zAxis.visibleRange = new NumberRange(
		centerZ - orbit,
		centerZ + orbit,
	);

	sciChart3DSurface.camera = new CameraController(wasmContext, {
		position: new Vector3(
			centerX - orbit * 0.82,
			centerY + orbit * 0.62,
			centerZ + orbit * 0.82,
		),
		target: new Vector3(centerX, centerY, centerZ),
	});
	sciChart3DSurface.worldDimensions = new Vector3(
		orbit * 2,
		orbit * 2,
		orbit * 2,
	);
};

export const initResonanceLatentChart = async (
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

	const trailMarker = new SpherePointMarker3D(wasmContext, {
		size: 0.08,
		fill: `${appTheme.VividSkyBlue}AA`,
	});

	const headMarker = new SpherePointMarker3D(wasmContext, {
		size: 0.18,
		fill: appTheme.VividPink,
	});

	const trailRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: trailSeries,
		pointMarker: trailMarker,
	});

	const headRenderable = new ScatterRenderableSeries3D(wasmContext, {
		dataSeries: headSeries,
		pointMarker: headMarker,
	});

	sciChart3DSurface.renderableSeries.add(trailRenderable, headRenderable);

	let surpriseScale = 1;
	let activeSymbol = "";

	const addData = (raw: Record<string, unknown>) => {
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

		headSeries.clear();
		headSeries.append(point.x, point.y, point.z);

		surpriseScale = Math.max(surpriseScale * 0.95, frame.surprise, 1e-6);
		headMarker.fill = surpriseFill(frame.surprise, surpriseScale);

		fitLatentCamera(sciChart3DSurface, wasmContext, trailSeries);
		sciChart3DSurface.invalidateElement();
	};

	return {
		sciChartSurface: sciChart3DSurface,
		wasmContext,
		addData,
	};
};
