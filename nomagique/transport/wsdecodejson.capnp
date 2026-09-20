using Go = import "/go.capnp";
@0xaf8e3ce634bf9266;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSDecodeJSON { data @0 :Data; }

interface WSDecodeJSON {
  write @0 (payload :WireWSDecodeJSON) -> stream;
  done @1 ();
}
