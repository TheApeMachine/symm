using Go = import "/go.capnp";
@0xcab1cd23a8e74561;
$Go.package("sequence");
$Go.import("github.com/theapemachine/symm/nomagique/data/sequence");

using import "../../runtime/status.capnp".Status;
using import "../../store/radix.capnp".Retained;

# Window retains the most recent readings of several quantities and hands them
# back as one matrix, newest row last.
#
# A relationship between two quantities cannot be read from a single update.
# Quantities are published on different frames, so a matrix whose rows are
# single updates has permanently disjoint support per producer, and anything
# measured from it describes the transport rather than what is being observed.
# A row here is one arrival of the whole reading, so quantities driven by
# different frames become comparable without any of them being asserted to
# have moved when they did not.
#
# Window is Retained: what it hands back is what it held when the evaluation
# began, so the node that reads it may also be the one that feeds it.
interface Window extends(Retained) {
  write @0 (value :List(Float64), present :List(Bool), span :Int32) -> stream;
  done @1 () -> (
    out     :List(Float64),
    rows    :Int32,
    cols    :Int32,
    full    :Bool,
    status  :Status
  );
}
