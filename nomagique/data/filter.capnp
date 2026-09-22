using Go = import "/go.capnp";
@0xc47e1a9b3d05f682;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

interface Filter {
  write @0 (
    data      :Data,
    path      :Text,
    operator  :Text,
    threshold :Float64,
    referencePath :Text
  ) -> stream;
  done @1 () -> Filtered;
}

struct Filtered {
  union { out @0 :Data; rejected @3 :Void; }
  passed @1 :Bool;
  status @2 :Status;
}
