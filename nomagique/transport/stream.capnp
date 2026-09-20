using Go = import "/go.capnp";
@0x923022abf2007907;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireStream { payload @0 :AnyPointer; }

interface Stream {
  write @0 (payload :WireStream) -> stream;
  done @1 ();
}
