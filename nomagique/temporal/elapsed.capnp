@0xb2990383054052bc;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

interface Elapsed {
  write @0 (timestamp :Int64) -> stream;
  done @1 () -> (out :Float64);
}
