@0xca744d9dba25119f;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

interface RLS {
  write @0 (x :Float64, target :Float64, lambda :Float64) -> stream;
  done @1 () -> (out :Float64);
}
