using Go = import "/go.capnp";
@0xc7d1e3f5a9b2c4d6;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface LinearPrediction {
  write @0 (x :Float64, slope :Float64, intercept :Float64) -> stream;
  done @1 () -> (out :Float64);
}
