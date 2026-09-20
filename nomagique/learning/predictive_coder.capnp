@0xc1ebbab287d30889;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");



interface PredictiveCoder {
  write @0 (features :List(Float64), reference :Float64, hasReference :Bool, step :Int64, time :Float64) -> stream;
  done @1 ();
}
