@0xad3c2a98512a5a4f;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Maximum {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 ();
}
