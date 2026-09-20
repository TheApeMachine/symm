@0xab2ca003c14203a8;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Bound {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
