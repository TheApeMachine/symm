using Go = import "/go.capnp";
@0xb9cf03445350777f;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireRoute { payload @0 :AnyPointer; destination @1 :Text; }

interface Route {
  write @0 (payload :WireRoute) -> stream;
  done @1 ();
}
