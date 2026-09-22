@0xf6e80498d6cfd6c5;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");



interface Attractor {
  write @0 (classes :List(Data), weights :List(UInt64)) -> stream;
  done @1 () -> (class :Data, prob :Float64, count :Int64);
}
