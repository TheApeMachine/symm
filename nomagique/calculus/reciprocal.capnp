@0xadcafcd802dbf517;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

# Reciprocal is 1/value. Zero has no reciprocal: the result is undefined, so
# nothing that reads out runs on it, rather than failing the evaluation or
# passing on an infinity.
interface Reciprocal {
  write @0 (value :Float64) -> stream;
  done @1 () -> Inverse;
}

struct Inverse {
  union {
    undefined @0 :Void;
    out       @1 :Float64;
  }
}
