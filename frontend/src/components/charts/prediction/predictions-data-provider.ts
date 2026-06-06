/*
The prediction chart plots forward forecasts against delayed ground truth.
These types describe a single point appended to one of the chart's series; the
series state itself lives in SciChart (see init-predictions-chart).
*/
export type PredictionSeriesKind = "actual" | "prediction" | "error";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	x: number;
	value: number;
};
