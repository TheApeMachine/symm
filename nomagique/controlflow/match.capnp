using Go = import "/go.capnp";
@0xbe418295a6f23db1;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

interface Match {
  write @0 (data :Data, pattern :Text) -> stream;
  done @1 () -> (out :Data, matched :Bool);
}
