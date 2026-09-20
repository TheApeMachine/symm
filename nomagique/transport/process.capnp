using Go = import "/go.capnp";
@0xb521b5a3da825bc1;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireProcess { executable @0 :Text; args @1 :List(Text); }

interface Process {
  write @0 (payload :WireProcess) -> stream;
  done @1 ();
}
