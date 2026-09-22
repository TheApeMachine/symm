using Go = import "/go.capnp";
@0xfa11b9a9d2dc590e;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Retained marks a primitive that hands back what it held before this
# evaluation began.
#
# Reading one is not a dependency on whatever writes it: the value is already
# there. That is what gives a graph defined feedback — a node may read a store
# it also writes, and the estimate it continues is the one it left behind.
interface Retained {
}

# Radix retains one value per key and reads it back, which is how a metric
# keeps its own local state for each symbol it observes.
#
# A write carrying a value retains it. A write carrying query reads the key
# back without retaining anything, so reading is never mistaken for observing.
interface Radix extends(Retained) {
  write @0 (key :Text, value :Data, query :Bool) -> stream;
  done @1 () -> (out :Data, found :Bool);
}
