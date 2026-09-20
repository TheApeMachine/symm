using Go = import "/go.capnp";
@0xfa11b9a9d2dc590e;
$Go.package("store");
$Go.import("nomagique/store");

interface Radix {
  write @0 (key :Text, value :Data) -> stream;
  done @1 () -> (out :Data, found :Bool);
}
