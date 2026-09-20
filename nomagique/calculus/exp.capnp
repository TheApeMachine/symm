@0xb8a8fe20d6b093d6;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Exp {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
