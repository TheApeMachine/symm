@0xe6824859c5da9e7d;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

interface Sign {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
