using Go = import "/go.capnp";
@0x8060fbf5b51245a1;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireJSONDecode { data @0 :Data; }

interface JSONDecode {
  write @0 (payload :WireJSONDecode) -> stream;
  done @1 ();
}
