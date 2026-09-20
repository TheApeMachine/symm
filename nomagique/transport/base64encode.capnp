using Go = import "/go.capnp";
@0x9d1dc5facc5148a2;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireBase64Encode { data @0 :Data; }

interface Base64Encode {
  write @0 (payload :WireBase64Encode) -> stream;
  done @1 ();
}
