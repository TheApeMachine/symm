using Go = import "/go.capnp";
@0xb521b5a3da825bc1;
$Go.package("transport");
$Go.import("github.com/theapemachine/symm/nomagique/transport");

interface Process {
  write @0 (data :Data, binary :Text, args :Text) -> stream;
  done @1 () -> (out :Data);
}
