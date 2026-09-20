using Go = import "/go.capnp";
@0x85a37dbf10b9d53f;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireTee { payload @0 :AnyPointer; offramps @1 :AnyPointer; }

interface Tee {
  write @0 (payload :WireTee) -> stream;
  done @1 ();
}
