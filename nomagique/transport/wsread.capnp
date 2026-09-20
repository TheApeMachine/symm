using Go = import "/go.capnp";
@0xde3a37ac2849ac5d;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireWSRead { connection @0 :AnyPointer; }

interface WSRead {
  write @0 (payload :WireWSRead) -> stream;
  done @1 ();
}
