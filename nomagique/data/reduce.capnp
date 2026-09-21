using Go = import "/go.capnp";
@0xe6913b7c084da2f5;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

interface Reduce {
  write @0 (value :Float64, operator :Text, flush :Bool) -> stream;
  done @1 () -> (
    out    :Float64,
    count  :Int64,
    ready  :Bool,
    status :Status
  );
}
