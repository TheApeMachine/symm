@0xe98988b3fd4bdb98;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Tanh {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
