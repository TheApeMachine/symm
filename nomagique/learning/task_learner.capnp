using Go = import "/go.capnp";
@0xabcdef1234567890;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface TaskLearner {
  write @0 (feature :Float64, target :Float64, observed :Bool) -> stream;
  done @1 () -> (out :Float64);
}
