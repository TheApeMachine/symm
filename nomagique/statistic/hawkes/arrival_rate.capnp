using Go = import "/go.capnp";
@0x800bd834b25cf3bc;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface ArrivalRate {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
