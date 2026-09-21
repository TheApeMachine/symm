@0xa171a732f5b3ae49;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Atanh {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
