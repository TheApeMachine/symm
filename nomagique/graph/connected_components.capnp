@0x81a3987fab9671e5;

using Go = import "/go.capnp";
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

interface ConnectedComponents {
  write @0 (
    fromNodes :List(Int64),
    toNodes :List(Int64),
  ) -> stream;

  done @1 () -> (
    componentCount :Int32,
    componentSizes :List(Int64),
    # Which component each node fell into, node id against component id.
    # The count says how many communities the graph broke into; this says
    # which one a given node belongs to, and without it a caller can only
    # learn that there are communities, never which one anything is in.
    members    :List(Int64),
    memberOf   :List(Int64),
  );
}
