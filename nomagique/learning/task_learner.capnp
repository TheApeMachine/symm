using Go = import "/go.capnp";
@0xabcdef1234567890;
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct WireTaskLearnerInput {
  features @0 :List(Float64);
  target @1 :Float64;
  observed @2 :Bool;
}

interface TaskLearner {
  write @0 (input :WireTaskLearnerInput) -> stream;
  done @1 ();
}
