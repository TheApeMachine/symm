package store

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Grid groups the frame's metric entries into the existing 12 confluence
regions. Grouping retains the entries and their producing measurements;
brightness remains a separate aggregate of their self-normalized movement.
*/
type Grid struct {
	*runtime.System
}

/*
RegionCount is the number of confluence regions the pins map metrics into.
*/
const RegionCount = 12

func NewGrid(ctx context.Context) *Grid {
	return &Grid{
		System: runtime.NewSystem(ctx, "grid"),
	}
}

/*
RegionMember refers to an original metric entry and its producing measurement.
The entry retains its key, raw value, signed SNM, unit and timescale; the
measurement retains the symbol, epoch, tick and observation interval. Neither
is copied or rewritten by grouping.
*/
type RegionMember struct {
	Measurement *data.Measurement
	Entry       *data.MetricEntry
}

/*
Region holds the individual members as well as the existing brightness
aggregate: Deformation is the sum of |SNM|, Counts is the member count, and
Brightness is their average. The aggregate does not replace member values.
*/
type Region struct {
	Key         uint8
	Deformation float64
	Brightness  float64
	Counts      int
	Members     []RegionMember
}

/*
Observe groups every metric of the frame's peers and returns all 12 regions,
brightest first, using the existing mean-|SNM| calculation and pin mapping.
A first observation with zero SNM is valid evidence and remains in the array.

An invalid frame, unreadable member, unpinned metric or empty region is a
contract error. No partial array is returned; the grid remains in error.
This checks region coverage, not that each producer emitted every declared
metric: that stronger source-level contract must not be inferred from Counts.
*/
func (grid *Grid) Observe(measurement *data.Measurement) []*Region {
	if grid.Error() != nil {
		return nil
	}

	if measurement == nil {
		grid.Error(errnie.Err(errnie.Validation, "[grid] nil signal frame", nil))
		return nil
	}

	peers := measurement.Peers()

	if err := measurement.Error(); err != nil {
		grid.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[grid] invalid signal frame: symbol=%s epoch=%d tick=%d",
				measurement.Label, measurement.Epoch, measurement.Tick),
			err,
		))

		return nil
	}

	regions := make([]*Region, RegionCount)

	for idx := range regions {
		regions[idx] = &Region{Key: uint8(idx)}
	}

	for _, peer := range peers {
		if err := grid.group(peer, regions); err != nil {
			grid.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[grid] cannot group signal frame: symbol=%s epoch=%d tick=%d",
					measurement.Label, measurement.Epoch, measurement.Tick),
				err,
			))

			return nil
		}
	}

	for _, region := range regions {
		if region.Counts == 0 {
			grid.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[grid] empty region R%02d: symbol=%s epoch=%d tick=%d",
					region.Key, measurement.Label, measurement.Epoch, measurement.Tick),
				nil,
			))

			return nil
		}

		region.Brightness = region.Deformation / float64(region.Counts)
	}

	slices.SortFunc(regions, func(first, second *Region) int {
		if first.Brightness > second.Brightness {
			return -1
		}

		if first.Brightness < second.Brightness {
			return 1
		}

		return 0
	})

	return regions
}

/*
group assigns one producer's original entries to their pinned regions. Read
errors are returned before any entry is dereferenced or used as evidence.
*/
func (grid *Grid) group(peer *data.Measurement, regions []*Region) error {
	if peer == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "[grid] nil signal peer", nil))
	}

	if err := peer.Error(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[grid] invalid signal peer: "+peer.Source, err,
		))
	}

	for entry := range peer.Read() {
		if entry == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[grid] nil metric entry: "+peer.Source, nil,
			))
		}

		if entry.Err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[grid] unreadable metric: "+peer.Source+"/"+entry.Key, entry.Err,
			))
		}

		if entry.Metric == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation, "[grid] nil metric: "+peer.Source+"/"+entry.Key, nil,
			))
		}

		key, isPinned := regionMetricPins[peer.Source][entry.Metric.Label()]

		if !isPinned {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				fmt.Sprintf("[grid] metric not pinned to a region: %s/%s",
					peer.Source, entry.Metric.Label()),
				nil,
			))
		}

		region := regions[key]
		region.Members = append(region.Members, RegionMember{Measurement: peer, Entry: entry})
		region.Deformation += math.Abs(entry.Metric.SNM())
		region.Counts++
	}

	return nil
}
