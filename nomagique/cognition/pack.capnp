@0xb02bd9a448461179;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Pack {
  write @0 (count :UInt64, mass :UInt64, writeStep :UInt64) -> stream;
  done @1 ();
}
