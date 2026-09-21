using Go = import "/go.capnp";
@0xb3d92c4e1f7a8506;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

interface Insert {
  write @0 (data :Data, path :Text, value :Float64) -> stream;
  done @1 () -> (out :Data, status :Status);
}
