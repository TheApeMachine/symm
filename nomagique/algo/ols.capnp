@0x9a95b9c3bf1635c6;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

struct OLSRow {
  values @0 :List(Float64);
}

interface OLS {
  write @0 (x :List(OLSRow), y :List(OLSRow)) -> stream;
  done @1 ();
}
