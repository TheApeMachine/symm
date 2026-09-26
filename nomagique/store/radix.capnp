using Go = import "/go.capnp";
@0xfa11b9a9d2dc590e;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Retained permits feedback writes after the evaluation has completed.
# Non-feedback inputs select what is read in the current observation. Inputs
# wired from descendants arrive in a second write after done; the retained
# value they change is available to subsequent observations.
interface Retained {
}

# Radix reads the requested keys from one immutable tree revision. An absent
# value list is a read. A supplied value list replaces all requested keys in
# one native radix transaction; its length must match and every slot must be
# nonempty. Empty slots mean no arrival and cannot be committed as values.
# The result always describes the revision before that write.

struct RadixSnapshot {
 format @0 :Text;
 keys @1 :List(Text);
 values @2 :List(Data);
 revision @3 :UInt64;
 contract @4 :Text; # Authored key/value semantics; restore requires an exact match.
}

using import "../runtime/snapshot.capnp".Checkpoint;

interface Radix extends(Retained, Checkpoint) {
  write @0 (key :List(Text), value :List(Data), path :Text, prefix :Bool, contract :Text) -> stream;
  done @1 () -> (out :List(Data), found :List(Bool), durable :Bool);
  measure @2 () -> (extent :UInt64);
}
