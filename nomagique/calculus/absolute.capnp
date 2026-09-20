@0x908d1a9c0ecc59c8;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Absolute {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
