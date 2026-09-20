@0xbab856b726fb9905;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface RelativeChange {
  write @0 (a :Float64) -> stream;
  done @1 ();
}
