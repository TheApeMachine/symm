using Go = import "/go.capnp";
@0xaaa9f194be266026;
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# GroupSum adds up the present values sharing a label. labels comes back in
# the order each label was first met, with its sum beside it. A label whose
# members were all absent is not a group with a sum of zero; it is left out.
# When no labels arrive there is nothing to group by, and no groups result.
interface GroupSum {
  write @0 (
    values  :List(Float64),
    present :List(Bool),
    labels  :List(Text)
  ) -> stream;
  done @1 () -> (
    labels :List(Text),
    sums   :List(Float64)
  );
}
