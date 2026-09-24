using Go = import "/go.capnp";
@0xa91402e3cdb7e270;
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Otsu splits labelled values into a strong class and the rest where the
# between-class variance peaks, so no count and no cutoff is chosen.
#
# Only positive values are candidates. A single candidate is the strong class
# on its own; candidates that are all equal are all strong. hot is the strong
# class's labels in label order, and out is the same labels as a JSON array.
# threshold is the smallest value in the strong class.
interface Otsu {
  write @0 (
    values :List(Float64),
    labels :List(Text)
  ) -> stream;
  done @1 () -> (
    hot       :List(Text),
    out       :Data,
    threshold :Float64
  );
}
