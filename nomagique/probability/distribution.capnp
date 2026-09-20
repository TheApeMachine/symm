@0xdf012110ce3ce448;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

struct Reading {
  probabilities @0 :List(Float64);
  winner @1 :Int64;
  confidence @2 :Float64;
  ambiguity @3 :Float64;
  sharpness @4 :Float64;
}

interface Distribution {
  write @0 (in :List(Float64)) -> stream;
  done @1 ();
}
