@0xc461bdf128bdf63b;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

# Log is the natural logarithm. A value that is not positive has none: the
# result is undefined, so nothing that reads out runs on it.
interface Log {
  write @0 (value :Float64) -> stream;
  done @1 () -> Logarithm;
}

struct Logarithm {
  union {
    undefined @0 :Void;
    out       @1 :Float64;
  }
}
