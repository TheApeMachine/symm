using Go = import "/go.capnp";
@0xe95a286819004042;
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

# Peak reads regions off an arranged landscape of points: the watershed basins
# of their authority, and whether that partition has settled.
#
# positions holds two numbers per point and authority one. fromNodes, toNodes
# and strength are the relationships read now. prior and known are each
# point's retained record, four numbers per point: the point it drains to (or
# -1), the peaks it drained to on the previous two evaluations, and its peak
# in the last partition that held (-1 wherever there is none yet).
#
# A point drains to a sympathetic neighbour of higher authority: one whose
# relationship with it is attraction and that the arrangement has pulled
# closer than one lattice cell, the distance a relationship of no strength is
# left at. Of those, it drains to the highest. A link is dropped when the
# relationship turns to repulsion, when the two drift a cell or more apart, or
# when the neighbour no longer stands higher. Points being merely near each
# other, or merely related, is not enough.
#
# Every point follows its links uphill to a peak. A region is everything that
# drains to one peak, so regions are bounded where the weakest points meet.
#
# state and index are every point's updated record, to be written back.
#
# A partition has held when every point drained to the same peak on the two
# evaluations before this one. Regions are only ever read from a partition
# that held, as it stood when the evaluation began: what this evaluation's
# relationships change takes effect on later evaluations, never on the regions
# this one publishes. While the arrangement is moving, the last partition that
# held stands; a partition in flight is never read. Until one has held,
# moving is reported and nothing is published. settled names each point's
# region by its peak's index, which is the peak's original coordinate.
interface Peak {
  write @0 (
    positions :List(Float64),
    authority :List(Float64),
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    strength  :List(Float64),
    prior     :List(Float64),
    known     :List(Bool)
  ) -> stream;
  done @1 () -> Watershed;
}

struct Watershed {
  state @0 :List(Float64);
  index @1 :List(Int64);

  union {
    moving  @2 :Void;
    settled @3 :List(Text);
  }
}
