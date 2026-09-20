@0xa171a732f5b3ae49;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Atanh {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
