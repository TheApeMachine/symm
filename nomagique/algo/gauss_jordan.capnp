@0xa30d63db64b1f5c7;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

struct GaussJordanRow {
  values @0 :List(Float64);
}

interface GaussJordan {
  write @0 (left :List(GaussJordanRow), right :List(GaussJordanRow)) -> stream;
  done @1 ();
}
