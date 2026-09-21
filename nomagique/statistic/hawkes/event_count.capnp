using Go = import "/go.capnp";
@0xcb55a8fefb87612f;
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

interface EventCount {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
