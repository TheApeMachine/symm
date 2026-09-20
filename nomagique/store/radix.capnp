using Go = import "/go.capnp";
@0xfa11b9a9d2dc590e;
$Go.package("store");
$Go.import("nomagique/store");

interface Radix {
  read @0 (key :Data) -> (value :AnyPointer, found :Bool);
  write @1 (key :Data, value :AnyPointer) -> (value :AnyPointer);
  identify @2 (key :Data, value :AnyPointer) -> (value :AnyPointer);
}
