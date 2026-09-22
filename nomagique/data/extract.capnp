using Go = import "/go.capnp";
@0xf935a1768bf2027d;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

interface Extract {
  write @0 (data :Data, path :Text, encoding :Text) -> stream;
  done @1 () -> Extracted;
}

struct Extracted {
  union { out @0 :Float64; json @3 :Data; text @4 :Text; missing @5 :Void; }
  found @1 :Bool;
  status @2 :Status;
}
