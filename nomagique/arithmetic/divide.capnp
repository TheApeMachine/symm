@0x98bc45995059e059;

using Go = import "/go.capnp";
$Go.package("arithmetic");
$Go.import("github.com/theapemachine/symm/nomagique/arithmetic");

# Divide is a/b. A zero divisor leaves the quotient undefined: nothing that
# reads out runs on it, so one undefined quotient withholds only what depends
# on it, never the rest of the evaluation, and no infinity is passed on.
interface Divide {
  write @0 (a :Float64, b :Float64) -> stream;
  done @1 () -> Quotient;
}

struct Quotient {
  union {
    undefined @0 :Void;
    out       @1 :Float64;
  }
}
