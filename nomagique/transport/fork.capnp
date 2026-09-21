using Go = import "/go.capnp";
@0x9c3995e0ef4d8001;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Fork {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
