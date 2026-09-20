using Go = import "/go.capnp";
@0xc43021d8a6551309;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireSHA256 { data @0 :Data; }

interface SHA256 {
  write @0 (payload :WireSHA256) -> stream;
  done @1 ();
}
