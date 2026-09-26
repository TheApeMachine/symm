import { PredictiveCodingCanvas } from "./predictive-coding-canvas";
import type { PredictionLayer } from "./prediction-chart";

export interface ResonanceReading {
	latent: number[];
	layers: PredictionLayer[];
	skillAverage: number;
	skillReadyAvg: boolean;
	precisionAverage: number;
	precisionReadyAvg: boolean;
	surprise: number;
	energy: number;
}
export interface ResonanceCanvasProps {
	reading?: ResonanceReading;
	forwardCurve?: number[];
	supportedHorizon?: number | string;
	calibrated?: boolean;
	resolvedSteps?: number | string;
	lastResolution?: { prediction: number; target: number; error: number };
	className?: string;
}

/* ResonanceCanvas renders the predictive node's actual latent hierarchy and
 resolved forecasts. Undefined skill remains undefined until the native estimator is ready. */
export const ResonanceCanvas = ({
	reading,
	forwardCurve,
	supportedHorizon,
	calibrated,
	resolvedSteps,
	lastResolution,
	className,
}: ResonanceCanvasProps) => (
	<PredictiveCodingCanvas
		className={className}
		latent={reading?.latent}
		layers={reading?.layers}
		forwardCurve={forwardCurve}
		skill={reading?.skillReadyAvg ? reading.skillAverage : undefined}
		relativePrecision={
			reading?.precisionReadyAvg ? reading.precisionAverage : undefined
		}
		energy={reading?.energy}
		surprise={reading?.surprise}
		samples={resolvedSteps}
		issued={lastResolution?.prediction}
		realized={lastResolution?.target}
		error={lastResolution?.error}
		horizon={
			supportedHorizon === undefined ? undefined : Number(supportedHorizon)
		}
		status={reading ? "settled" : "Awaiting a complete metric cut"}
		skillStatus={
			reading?.skillReadyAvg ? "measured" : "Awaiting resolved forecasts"
		}
		forecast={
			calibrated && forwardCurve?.length
				? forwardCurve[forwardCurve.length - 1]
				: undefined
		}
	/>
);
