@0xcf9c1fe749579fb8;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

interface Delay {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
