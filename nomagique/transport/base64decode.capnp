using Go = import "/go.capnp";
@0xcccd5dccee73e169;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireBase64Decode { data @0 :Text; }

interface Base64Decode {
  write @0 (payload :WireBase64Decode) -> stream;
  done @1 ();
}
