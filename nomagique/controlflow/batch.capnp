using Go = import "/go.capnp";
@0xe19468972b5341cf;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

interface Batch {
  write @0 (item :Data, size :Int64, flush :Bool) -> stream;
  done @1 () -> (out :Data, count :Int64, ready :Bool);
}
