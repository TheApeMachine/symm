using Go = import "/go.capnp";
@0xa66153462ea72a0d;
$Go.package("graph");
$Go.import("github.com/theapemachine/symm/nomagique/graph");

# Complete is the complete graph over the nodes present now: one edge for
# every pair of present nodes, lower node first.
#
# pairs names each edge by its position in the full node-by-node matrix,
# from × count + to, where count is the length of present. It is a stable
# address for anything retained per pair.
interface Complete {
  write @0 (present :List(Bool)) -> stream;
  done @1 () -> (
    fromNodes :List(Int64),
    toNodes   :List(Int64),
    pairs     :List(Int64)
  );
}
