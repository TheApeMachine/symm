using Go = import "/go.capnp";
@0xf935a1768bf2027d;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

interface Extract {
  write @0 (data :Data, path :Text) -> stream;
  done @1 () -> (out :Float64, found :Bool, status :Status);
}
