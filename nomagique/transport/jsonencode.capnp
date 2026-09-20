using Go = import "/go.capnp";
@0xa9aa4e1647796f92;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireJSONEncode { payload @0 :AnyPointer; }

interface JSONEncode {
  write @0 (payload :WireJSONEncode) -> stream;
  done @1 ();
}
