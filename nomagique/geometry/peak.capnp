using Go = import "/go.capnp";
@0xe95a286819004042;
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

# Peak reads regions off an arranged landscape of points: the watershed basins
# of their authority, and whether that partition has settled.
#
# positions holds two numbers per point and authority one. fromNodes, toNodes
# and strength are the relationships read now. prior and known are each
# point's retained record, three numbers per point: the point it drains to (or
# -1), the peak it drained to on the previous evaluation, and its peak in the
# last partition that settled (or -1).
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
# state and index are every point's updated record, to be written back. The
# partition has settled when every point drains to the same peak it did on
# the previous evaluation. Regions are always read from the last partition
# that settled: until one has, nothing is published, and while the
# arrangement moves again (as every impulse makes it do) the settled regions
# stand rather than regions of a map in flight. settled names each point's
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
