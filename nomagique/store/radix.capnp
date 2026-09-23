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
interface Radix extends(Retained) {
  write @0 (key :List(Text), value :List(Data)) -> stream;
  done @1 () -> (out :List(Data), found :List(Bool));
}
