/*
The prediction chart plots the live "prediction" signal's confidence over time.
These types describe a single point appended to one of the chart's series; the
series state itself lives in SciChart (see init-predictions-chart).
*/
export type PredictionSeriesKind = "average" | "prediction" | "error";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	x: number;
	value: number;
};
