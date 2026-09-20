@0xd8ca2d377e4629b0;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Minimum {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 () -> (out :Float64);
}
