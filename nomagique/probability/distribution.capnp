@0xdf012110ce3ce448;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Distribution {
  write @0 (in :Float64) -> stream;
  done @1 () -> (
    out :Float64,
    winner :Int64,
    confidence :Float64,
    ambiguity :Float64,
    sharpness :Float64
  );
}
