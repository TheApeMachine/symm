using Go = import "/go.capnp";
@0xa7a831831c2a1c7a;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSEncodeJSON { payload @0 :AnyPointer; }

interface WSEncodeJSON {
  write @0 (payload :WireWSEncodeJSON) -> stream;
  done @1 ();
}
