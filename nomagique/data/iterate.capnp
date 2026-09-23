using Go = import "/go.capnp";
@0xd58b06f2c9143ae7;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

# indexPath optionally inserts the original zero-based collection index into
# each projected document. It requires envelope mode and an unoccupied path.
interface Iterate {
  write @0 (data :List(Data), path :Text, envelope :Bool, indexPath :Text) -> stream;
  done @1 () -> (
    out    :Data,
    index  :Int64,
    count  :Int64,
    last   :Bool,
    found  :Bool,
    pending :UInt64,
    ignored :UInt64,
    status :Status
  );
}
