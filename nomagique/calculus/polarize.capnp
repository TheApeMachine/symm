@0xa0ad9f64dd8c7603;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Polarize {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 ();
}
