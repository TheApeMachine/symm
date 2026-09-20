using Go = import "/go.capnp";
@0xa7a831831c2a1c7a;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSEncodeJSON {
  write @0 (in :Data) -> stream;
  done @1 () -> (out :Data);
}
