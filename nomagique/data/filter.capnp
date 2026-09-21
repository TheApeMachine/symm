using Go = import "/go.capnp";
@0xc47e1a9b3d05f682;
$Go.package("data");
$Go.import("nomagique/data");

using import "../runtime/status.capnp".Status;

interface Filter {
  write @0 (
    data      :Data,
    path      :Text,
    operator  :Text,
    threshold :Float64
  ) -> stream;
  done @1 () -> (out :Data, passed :Bool, status :Status);
}
