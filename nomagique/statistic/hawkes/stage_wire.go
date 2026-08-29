package hawkes

/*
encodeMomentBatch flattens aligned x/y count streams into one wire batch.
*/
func encodeMomentBatch(xStream, yStream []float64) []float64 {
	if len(xStream) != len(yStream) || len(xStream) < 2 {
		return nil
	}

	batch := make([]float64, 0, len(xStream)+len(yStream))
	batch = append(batch, xStream...)
	batch = append(batch, yStream...)

	return batch
}

/*
encodeFitBatch flattens x/y arrival-time streams into one wire batch.
*/
func encodeFitBatch(xTimesSec, yTimesSec []float64) []float64 {
	if len(xTimesSec)+len(yTimesSec) < 2 {
		return nil
	}

	batch := make([]float64, 0, len(xTimesSec)+len(yTimesSec))
	batch = append(batch, xTimesSec...)
	batch = append(batch, yTimesSec...)

	return batch
}
