using Go = import "/go.capnp";
@0xc626270c60f00576;
$Go.package("data");
$Go.import("nomagique/data");

struct WireSeries {
  payload @0 :AnyPointer;
}

interface Series {
  write @0 (series :WireSeries) -> stream;
  done @1 ();
}
