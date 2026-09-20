using Go = import "/go.capnp";
@0xeec260166dcfeaf8;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireHeaderAuth { headers @0 :AnyPointer; key @1 :Text; value @2 :Text; }

interface HeaderAuth {
  write @0 (payload :WireHeaderAuth) -> stream;
  done @1 ();
}
