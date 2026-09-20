using Go = import "/go.capnp";
@0xfb9dbf8960eb25d6;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSBatch { messages @0 :AnyPointer; }

interface WSBatch {
  write @0 (payload :WireWSBatch) -> stream;
  done @1 ();
}
