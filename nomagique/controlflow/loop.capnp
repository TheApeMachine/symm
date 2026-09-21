using Go = import "/go.capnp";
@0xb3033614ee2b651b;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

interface Loop {
  write @0 (data :Data, active :Bool, limit :Int64) -> stream;
  done @1 () -> (out :Data, index :Int64, done :Bool);
}
