using Go = import "/go.capnp";
@0x9899919ee1ccdded;
$Go.package("transport");
$Go.import("nomagique/transport");

interface Pace {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
