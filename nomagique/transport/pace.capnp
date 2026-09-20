using Go = import "/go.capnp";
@0x9899919ee1ccdded;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WirePace { payload @0 :AnyPointer; interval @1 :Int64; }

interface Pace {
  write @0 (payload :WirePace) -> stream;
  done @1 ();
}
