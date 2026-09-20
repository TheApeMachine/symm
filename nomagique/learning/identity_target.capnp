@0xb56e0481a4af6c09;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface IdentityTarget {
  write @0 (past :Float64, current :Float64) -> stream;
  done @1 () -> (out :Float64);
}
