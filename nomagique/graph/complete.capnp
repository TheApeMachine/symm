using Go = import "/go.capnp";
@0xa66153462ea72a0d;
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

# Complete is the complete graph over the nodes present now: one edge for
# every pair of present nodes, lower node first.
#
# With reach, every present node is also joined to every node that is not
# present, so a relationship can be read where one end reported and the other
# did not. joint says, per edge, whether both ends are present.
#
# pairs names each edge by its position in the full node-by-node matrix,
# from × count + to, where count is the length of present. It is a stable
# address for anything retained per pair.
interface Complete {
  write @0 (present :List(Bool), reach :Bool) -> stream;
  done @1 () -> (
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    pairs     :List(Int64),
    joint     :List(Bool)
  );
}
