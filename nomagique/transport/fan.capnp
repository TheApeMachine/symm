using Go = import "/go.capnp";
@0xd844680ffcfff91c;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Fan {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
