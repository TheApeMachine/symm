using Go = import "/go.capnp";
@0xf790ed006f8391af;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireJoin { payloads @0 :AnyPointer; }

interface Join {
  write @0 (payload :WireJoin) -> stream;
  done @1 ();
}
