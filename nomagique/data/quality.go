package data

import (
	"context"
	"strconv"
)

// QualityServer implements Quality_Server from the capnp schema.
type QualityServer struct{}

func NewQualityServer() *QualityServer {
	return &QualityServer{}
}

func (s *QualityServer) Evaluate(ctx context.Context, call Quality_evaluate) error {
	facts, err := call.Args().Facts()
	if err != nil {
		return err
	}

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	reading, err := res.NewReading()
	if err != nil {
		return err
	}

	reading.SetEstimated(facts.HasSupport() || facts.HasDivergence() || facts.HasMahalanobis())
	reading.SetMaturity(1.0)

	if facts.HasDivergence() && facts.HasNoise() && facts.NoiseVariance() > 0 {
		reading.SetSnr(facts.Divergence() * facts.Divergence() / facts.NoiseVariance())
		reading.SetSnrDefined(true)
	}

	if facts.HasSupport() {
		reading.SetMaturity(0.0)

		if facts.Support() > 1 {
			reading.SetMaturity(1.0 - 1.0/facts.Support())

			if facts.HasMahalanobis() && facts.MahalanobisSNR() >= 0 {
				reading.SetSnr(facts.MahalanobisSNR())
				reading.SetSnrDefined(true)
			}
		}
	}

	if !facts.HasSupport() && facts.HasMaturity() {
		reading.SetMaturity(facts.Maturity())
	}

	return nil
}

// FactsFromMetadata constructs WireQualityFacts from a string metadata map.
// This is typically called prior to dispatching to the QualityServer.
func FactsFromMetadata(metadata map[string]string, facts WireQualityFacts) error {
	if metadata == nil {
		return nil
	}

	if val, ok := metadata["support"]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.SetSupport(parsed)
			facts.SetHasSupport(true)
		}
	}

	if val, ok := metadata["divergence"]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.SetDivergence(parsed)
			facts.SetHasDivergence(true)
		}
	}

	if val, ok := metadata["noise_variance"]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.SetNoiseVariance(parsed)
			facts.SetHasNoise(true)
		}
	}

	if val, ok := metadata["mahalanobis_snr"]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.SetMahalanobisSNR(parsed)
			facts.SetHasMahalanobis(true)
		}
	}

	if val, ok := metadata["maturity"]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.SetMaturity(parsed)
			facts.SetHasMaturity(true)
		}
	}

	return nil
}
