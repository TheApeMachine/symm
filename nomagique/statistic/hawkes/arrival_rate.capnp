using Go = import "/go.capnp";
@0x800bd834b25cf3bc;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

struct WireArrivalRate {
  payload @0 :AnyPointer;
}

interface ArrivalRate {
  write @0 (view :WireArrivalRate) -> stream;
  done @1 ();
}
