@0xdec0cddc8aa8ae23;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Latest owns one causally stamped value per named member. Updating a member
# replaces only that member. A new epoch starts an empty cohort. Members are
# emitted in insertion order; values and stamps always have matching indices.
interface Latest {
  write @0 (key :Text, value :Float64, epoch :Int64, sequence :Int64) -> stream;
  done @1 () -> (keys :List(Text), values :List(Float64), epoch :Int64,
                sequences :List(Int64));
}
