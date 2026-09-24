@0xd0d4d1eefe7c9bcc;

using Go = import "/go.capnp";
$Go.package("calculus");
$Go.import("github.com/theapemachine/symm/nomagique/calculus");

# Sqrt is the real square root. A negative value has none: the result is
# undefined, so nothing that reads out runs on it, rather than reading a zero
# or failing the whole evaluation over a quantity that is simply not defined
# yet (a standard error with too few observations, say).
interface Sqrt {
  write @0 (value :Float64) -> stream;
  done @1 () -> Root;
}

struct Root {
  union {
    undefined @0 :Void;
    out       @1 :Float64;
  }
}
