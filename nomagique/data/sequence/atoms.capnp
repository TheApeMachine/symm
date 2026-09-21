using Go = import "/go.capnp";
@0xcab1cd23a8e74561;
$Go.package("sequence");
$Go.import("github.com/theapemachine/symm/nomagique/data/sequence");

struct WireWindow {
  payload @0 :AnyPointer;
  size @1 :Int32;
}

struct WireTail {
  payloads @0 :AnyPointer;
  size @1 :Int32;
}

struct WireAt {
  payloads @0 :AnyPointer;
  index @1 :Int32;
}

struct WireValues {
  payloads @0 :AnyPointer;
}

struct WireOrder {
  payloads @0 :AnyPointer;
}

struct WireAppend {
  slice @0 :AnyPointer;
  item @1 :AnyPointer;
}

interface Window {
  execute @0 (window :WireWindow) -> (results :AnyPointer);
}

interface Tail {
  execute @0 (tail :WireTail) -> (results :AnyPointer);
}

interface At {
  execute @0 (at :WireAt) -> (result :AnyPointer);
}

interface Values {
  execute @0 (values :WireValues) -> (results :AnyPointer);
}

interface Order {
  execute @0 (order :WireOrder) -> (results :AnyPointer);
}

interface Append {
  execute @0 (append :WireAppend) -> (results :AnyPointer);
}
