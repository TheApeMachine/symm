@0xe1018345b743c62c;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Histogram bins a set of values. values is a JSON array of numbers. The bin
# width is Freedman-Diaconis, 2 IQR / n^(1/3), so it comes from the spread of
# the values themselves. out is a JSON array of {id, lower, upper, count}.
# Values with no spread, or too few to have quartiles, have no distribution to
# draw, and it is idle.
interface Histogram {
  write @0 (values :Data) -> stream;
  done @1 () -> Binned;
}

struct Binned {
  union {
    idle @0 :Void;
    out  @1 :Data;
  }
}
