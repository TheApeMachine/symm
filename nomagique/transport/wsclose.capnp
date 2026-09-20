using Go = import "/go.capnp";
@0x9752b0b9c04840a6;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSClose { connection @0 :AnyPointer; }

interface WSClose {
  write @0 (payload :WireWSClose) -> stream;
  done @1 ();
}
