using Go = import "/go.capnp";
@0xd844680ffcfff91c;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireFan { payload @0 :AnyPointer; branches @1 :AnyPointer; }

interface Fan {
  write @0 (payload :WireFan) -> stream;
  done @1 ();
}
