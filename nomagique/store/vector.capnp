using Go = import "/go.capnp";
@0xd7463b0658d5a683;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "radix.capnp".Retained;

# Vector retains numeric records of a fixed width, addressed by position.
#
# Writing index and values stores one record per index: values carries width
# numbers for each index in the same order. Positions never written are
# unknown, never zero, and found says so for each record read.
#
# read names the records to hand back. When read is not wired, done hands back
# the whole vector. Either way what is handed back is what the vector held
# when the evaluation began, so a descendant can write the next state back
# into it as feedback.
#
# scope names the series the records belong to; it gathers, and every scope
# written together must agree. Records written under one scope are never
# handed out under another: a new scope starts from nothing. No scope arriving
# keeps the current series; an unwired scope is one series.
interface Vector extends(Retained) {
  write @0 (
    width  :UInt32,
    read   :List(Int64),
    index  :List(Int64),
    values :List(Float64),
    scope  :List(Text)
  ) -> stream;
  done @1 () -> (
    values  :List(Float64),
    found   :List(Bool),
    records :Int64
  );
}
