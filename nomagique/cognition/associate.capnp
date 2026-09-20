@0xf7955c5d36bd7258;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Associate {
  write @0 (current :Data) -> stream;
  done @1 () -> (
    precursor :Data,
    current :Data
  );
}
