using Go = import "/go.capnp";
@0xd58b06f2c9143ae7;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

interface Iterate {
  write @0 (data :Data, path :Text) -> stream;
  done @1 () -> (
    out    :Data,
    index  :Int64,
    count  :Int64,
    last   :Bool,
    found  :Bool,
    status :Status
  );
}
