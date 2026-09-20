@0xb1a0f1ee286540fe;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface DirectionalTarget {
  write @0 (past :Float64, current :Float64) -> stream;
  done @1 () -> (out :Float64);
}
