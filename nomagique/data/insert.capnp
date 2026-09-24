using Go = import "/go.capnp";
@0xb3d92c4e1f7a8506;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

# Unique insertion returns inserted=false for an identical existing value and
# rejects a conflicting value. It never silently replaces a unique fact.
# encoding=uint64 selects unsigned without a Float64 conversion and excludes
# json. encoding=text inserts text as a string and excludes json. Empty
# encoding retains the existing number/explicit-JSON input contract.
interface Insert {
  write @0 (data :Data, path :Text, value :Float64, json :Data, unique :Bool, encoding :Text, unsigned :UInt64, text :Text) -> stream;
  done @1 () -> (out :Data, status :Status, inserted :Bool);
}
