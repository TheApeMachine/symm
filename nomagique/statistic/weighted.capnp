@0xb3e1f7a25c9d4068;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Weighted summarises a set of observations that did not all carry the same
# authority. Each value is weighted by the evidence behind it, so a reading
# formed from two overlapping returns does not count as much as one formed
# from two hundred.
#
# Several producers land on value and weight, so a set grows by wiring one
# more in rather than by widening the schema.
interface Weighted {
  write @0 (
    value  :List(Float64),
    weight :List(Float64)
  ) -> stream;
  done @1 () -> (
    mean      :Float64,
    variance  :Float64,
    total     :Float64,
    count     :Float64,
    effective :Float64
  );
}
