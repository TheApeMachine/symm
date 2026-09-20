using Go = import "/go.capnp";
@0xec12f25c9a429685;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireGate { payload @0 :AnyPointer; condition @1 :Text; }

interface Gate {
  write @0 (payload :WireGate) -> stream;
  done @1 ();
}
