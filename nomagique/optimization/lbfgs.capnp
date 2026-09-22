@0xa1025bc2d2c2419f;

using Go = import "/go.capnp";
$Go.package("optimization");
$Go.import("github.com/theapemachine/symm/nomagique/optimization");

using import "../store/radix.capnp".Retained;

# LBFGS is an iterate stepper, not a solver. It proposes a point, is told
# what the objective was worth there, and proposes the next one. The
# objective is another node entirely, and the graph closes the loop, which is
# what keeps this node free of any knowledge of what it is minimizing.
#
# It is not told which point the value belongs to, because it already knows:
# the value answers the point it last proposed. A caller that could name a
# different one could silently corrupt the curvature history.
#
# It is Retained, so reading its proposal is reading what it held before this
# evaluation rather than a dependency on this one. That is what lets the
# objective be downstream of the optimizer and the optimizer downstream of
# the objective without the graph closing a cycle.
interface LBFGS extends(Retained) {
  write @0 (
    fVal      :Float64,
    gradient  :List(Float64),
    seed      :List(Float64),
    memory    :Int32,
    tolerance :Float64,
  ) -> stream;

  done @1 () -> (
    x          :List(Float64),
    fVal       :Float64,
    iterations :Int32,
    converged  :Bool,
  );
}
