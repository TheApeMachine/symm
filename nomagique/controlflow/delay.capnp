using Go = import "/go.capnp";
@0xa93158c38749b5d2;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

interface Delay {
  write @0 (data :Data, millis :Int64) -> stream;
  done @1 () -> (out :Data, ready :Bool);
}
