@0x88ffed6b12683d48;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Lookahead {
  write @0 (prefix :Data) -> stream;
  done @1 () -> (seq :Data, logP :Float64);
}
