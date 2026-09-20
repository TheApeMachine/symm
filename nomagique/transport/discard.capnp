using Go = import "/go.capnp";
@0xda7d76c759c3aa9e;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireDiscard { payload @0 :AnyPointer; }

interface Discard {
  write @0 (payload :WireDiscard) -> stream;
  done @1 ();
}
