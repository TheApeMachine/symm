@0xad5c95d74fb1c123;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface DeltaTarget {
  write @0 (past :Float64, current :Float64) -> stream;
  done @1 () -> (out :Float64);
}
