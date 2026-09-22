@0x9cbcdce02ecb8458;

using Go = import "/go.capnp";
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Element takes one value out of a list by position.
#
# It reports whether the position existed rather than answering zero for one
# that did not, because a list that is shorter than expected and a list whose
# value happens to be zero are different facts about the producer upstream.
interface Element {
  write @0 (
    values :List(Float64),
    index  :Int32,
  ) -> stream;

  done @1 () -> (
    out   :Float64,
    found :Bool,
  );
}
