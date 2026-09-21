@0xc2a9900798d217ef;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Negate {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
