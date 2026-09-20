using Go = import "/go.capnp";
@0xb8e2f4a6c1d3e5f7;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface Counterfactual {
  write @0 (treatment :Float64, outcome :Float64, confounder :Float64) -> stream;
  done @1 () -> (effect :Float64, out :Float64);
}
