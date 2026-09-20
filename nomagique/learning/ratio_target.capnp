@0xe83153c554030981;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface RatioTarget {
  write @0 (past :Float64, current :Float64) -> stream;
  done @1 () -> (out :Float64);
}
