using Go = import "/go.capnp";
@0xc7b587951848ea4b;
$Go.package("transport");
$Go.import("nomagique/transport");

interface WSWrite {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
