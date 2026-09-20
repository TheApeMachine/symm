@0xacad158a8debf50f;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct CounterfactualRow {
  values @0 :List(Float64);
}

interface Counterfactual {
  write @0 (history :List(CounterfactualRow), factualRow :List(Float64)) -> stream;
  done @1 ();
}
