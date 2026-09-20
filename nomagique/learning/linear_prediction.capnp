@0xb372af8b7ded291d;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");


interface LinearPrediction {
  write @0 (coefficients :List(Float64), rawRow :List(Float64)) -> stream;
  done @1 ();
}
