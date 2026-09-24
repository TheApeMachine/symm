using Go = import "/go.capnp";
@0x9bee3ab11b87719e;
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

# Relaxation moves points on a plane toward the distances their relationships
# ask for, one stress step per relationship read now.
#
# authority has one entry per point and fixes how many points there are.
# positions and known are where each point was left, two numbers per point;
# a point never placed starts at its original coordinate on the square lattice
# of that many points: (i mod width, i div width).
#
# Each relationship pulls or pushes its two points toward its target distance.
# Its pull is |s| times the pair's combined authority, and it corrects
# pull/(1+pull) of the residual: a stronger relationship between better
# evidenced points closes more of it, a weak one between immature points
# barely moves them, and none overshoots. The correction is split by
# authority: each end moves by the other end's share of their combined
# authority, so a weak point travels to a strong one rather than the other way
# round. Two points with no authority between them have no evidence to move by.
#
# positions and index are every point's new place, to be written back.
interface Relaxation {
  write @0 (
    positions :List(Float64),
    known     :List(Bool),
    authority :List(Float64),
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    strength  :List(Float64),
    distance  :List(Float64)
  ) -> stream;
  done @1 () -> (
    positions :List(Float64),
    index     :List(Int64)
  );
}
