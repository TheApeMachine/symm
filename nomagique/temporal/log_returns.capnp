@0x98243cbf8a71241e;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

interface LogReturns {
  write @0 (a :Float64) -> stream;
  done @1 () -> (out :Float64);
}
