using Go = import "/go.capnp";
@0x9c3995e0ef4d8001;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireFork { payload @0 :AnyPointer; routes @1 :List(Text); }

interface Fork {
  write @0 (payload :WireFork) -> stream;
  done @1 ();
}
