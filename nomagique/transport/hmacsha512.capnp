using Go = import "/go.capnp";
@0xbc1534465ace77d2;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireHMACSHA512 { message @0 :Data; secret @1 :Data; }

interface HMACSHA512 {
  write @0 (payload :WireHMACSHA512) -> stream;
  done @1 ();
}
