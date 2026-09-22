@0x8824b5999cefc8be;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

using import "memory.capnp".Memory;

interface Reinforce {
  write @0 (contextBytes :Data, classBytes :Data, memory :Memory) -> stream;
  done @1 () -> (out :Data);
}
