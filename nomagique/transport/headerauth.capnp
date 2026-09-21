using Go = import "/go.capnp";
@0xeec260166dcfeaf8;
$Go.package("transport");
$Go.import("nomagique/transport");

interface HeaderAuth {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
