@0x8d86bc2a425ee232;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface ParseBasinKey {
  write @0 (key :Data) -> stream;
  done @1 () -> (
    class :Data,
    contextBytes :Data,
    ok :Bool
  );
}
