using Go = import "/go.capnp";
@0xc7b587951848ea4b;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSWrite { connection @0 :AnyPointer; payload @1 :Data; }

interface WSWrite {
  write @0 (payload :WireWSWrite) -> stream;
  done @1 ();
}
