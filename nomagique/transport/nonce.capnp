using Go = import "/go.capnp";
@0xd0fb0fb33ce02b1c;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireNonce { }

interface Nonce {
  write @0 (payload :WireNonce) -> stream;
  done @1 ();
}
