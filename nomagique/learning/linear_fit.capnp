using Go = import "/go.capnp";
@0xa4f8b2c1d3e5f7a9;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface LinearFit {
  write @0 (x :Float64, y :Float64) -> stream;
  done @1 () -> (slope :Float64, intercept :Float64, r2 :Float64);
}
