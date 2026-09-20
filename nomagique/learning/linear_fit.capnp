@0xfb927e5bb5633aca;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct LinearFitRow {
  values @0 :List(Float64);
}

interface LinearFit {
  write @0 (rows :List(LinearFitRow)) -> stream;
  done @1 ();
}
