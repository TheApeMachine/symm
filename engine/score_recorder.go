package engine

/*
ScoreRecorder normalizes raw signal strength against capped per-symbol history.
*/
type ScoreRecorder struct {
	Calibrator     PredictionCalibrator
	ConfidenceHist []float64
	LiveScore      float64
	HistoryCap     int
}

/*
NewScoreRecorder creates an empty recorder with the given history capacity.
*/
func NewScoreRecorder(params CalibrationParams, historyCap int) ScoreRecorder {
	return ScoreRecorder{
		Calibrator:     NewPredictionCalibrator(params),
		ConfidenceHist: make([]float64, 0, historyCap),
		HistoryCap:     historyCap,
	}
}

/*
Record maps rawScore into unit scale, appends history, and optionally feeds gauge scan.
*/
func (recorder *ScoreRecorder) Record(rawScore float64, gaugeScan *GaugeScan) float64 {
	if rawScore <= 0 {
		return 0
	}

	normalized := recorder.Calibrator.NormalizeConfidence(rawScore, recorder.ConfidenceHist)
	recorder.LiveScore = normalized
	recorder.ConfidenceHist = append(recorder.ConfidenceHist, rawScore)

	if len(recorder.ConfidenceHist) > recorder.HistoryCap {
		recorder.ConfidenceHist = recorder.ConfidenceHist[len(recorder.ConfidenceHist)-recorder.HistoryCap:]
	}

	if gaugeScan != nil {
		gaugeScan.ObserveGaugeScore(normalized)
	}

	return normalized
}

/*
RecordCalibrated scales rawScore by the live calibrator and records it.
*/
func (recorder *ScoreRecorder) RecordCalibrated(rawScore float64, gaugeScan *GaugeScan) float64 {
	scale := recorder.Calibrator.Scale()

	if scale <= 0 || rawScore <= 0 {
		return 0
	}

	return recorder.Record(rawScore*scale, gaugeScan)
}
