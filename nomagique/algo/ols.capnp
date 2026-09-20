@0x9a95b9c3bf1635c6;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

interface OLS {
  write @0 (x :Float64, y :Float64) -> stream;
  done @1 () -> (out :Float64);
}
