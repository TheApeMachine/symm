using Go = import "/go.capnp";
@0xfa11b9a9d2dc590e;
$Go.package("store");
$Go.import("nomagique/store");

# Radix retains one value per key and reads it back, which is how a metric
# keeps its own local state for each symbol it observes.
#
# A write carrying a value retains it. A write carrying query reads the key
# back without retaining anything, so reading is never mistaken for observing.
interface Radix {
  write @0 (key :Text, value :Data, query :Bool) -> stream;
  done @1 () -> (out :Data, found :Bool);
}
