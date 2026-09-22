package temporal

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
VelocityServer owns how fast an observable is moving.

The rate is the slope of a least-squares fit across the observations seen so
far, not the difference between the last two. Two points always produce a
slope, and it is never possible to tell from them whether the move was real or
was the noise the series already carried. Fitting reports the slope together
with the confidence behind it, so a caller can tell those apart.
*/
type VelocityServer struct {
	count       float64
	sumTime     float64
	sumValue    float64
	sumTimeSq   float64
	sumProduct  float64
	sumValueSq  float64
	slope       float64
	slopeSNR    float64
	slopeIsReal bool
}

func NewVelocity() *VelocityServer {
	return &VelocityServer{}
}

/*
Write admits one observation and refits the slope across everything seen.
*/
func (server *VelocityServer) Write(ctx context.Context, call Velocity_write) error {
	value := call.Args().Val()
	at := call.Args().Ts()

	server.count++
	server.sumTime += at
	server.sumValue += value
	server.sumTimeSq += at * at
	server.sumProduct += at * value
	server.sumValueSq += value * value

	server.fit()
	return nil
}

/*
Done reports the rate, the confidence behind it, and whether a rate could be
formed at all.
*/
func (server *VelocityServer) Done(ctx context.Context, call Velocity_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"temporal: alloc velocity results failed",
			err,
		))
	}

	results.SetOut(server.slope)
	results.SetSnr(server.slopeSNR)
	results.SetDefined(server.slopeIsReal)

	server.slope, server.slopeSNR, server.slopeIsReal = 0, 0, false
	return nil
}

/*
fit forms the least-squares slope and how large it is against its own
uncertainty.

A slope needs two distinct instants to exist and three to have an uncertainty,
so below that it is reported as undefined rather than as a rate of zero.
*/
func (server *VelocityServer) fit() {
	server.slopeIsReal = false

	if server.count < 2 {
		return
	}

	// Centred sums: the same fit, formed where the observations actually sit
	// rather than against an origin they may be far from.
	spreadTime := server.sumTimeSq - (server.sumTime*server.sumTime)/server.count
	spreadValue := server.sumValueSq - (server.sumValue*server.sumValue)/server.count
	together := server.sumProduct - (server.sumTime*server.sumValue)/server.count

	if spreadTime == 0 {
		return
	}

	server.slope = together / spreadTime
	server.slopeIsReal = true

	if server.count < 3 {
		return
	}

	// What the fitted line did not account for. A line through every point
	// leaves nothing, and its slope is then certain rather than unmeasured.
	unexplained := spreadValue - server.slope*together
	slopeVariance := (unexplained / (server.count - 2)) / spreadTime

	server.slopeSNR = (server.slope * server.slope) / slopeVariance
}
