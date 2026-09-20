using Go = import "/go.capnp";
@0x8de09eaaee89afbc;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireHMACSHA256 { message @0 :Data; secret @1 :Data; }

interface HMACSHA256 {
  write @0 (payload :WireHMACSHA256) -> stream;
  done @1 ();
}
