using Go = import "/go.capnp";
@0xec12f25c9a429685;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Gate {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
