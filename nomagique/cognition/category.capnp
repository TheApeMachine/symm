@0xca42f2042d2591b2;
using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");
using Record = import "../types/record.capnp".Record;

struct CategoryReading {
  epoch @0 :Int64;
  sequence @1 :Int64;
  symbol @2 :Text;
  dominant @3 :Text;
  uncertainty @4 :Float64;
  categories @5 :List(CategoryEvidence);
}

struct CategoryEvidence {
  name @0 :Text;
  strength @1 :Float64;
  confidence @2 :Float64;
  surprisal @3 :Float64;
  supporting @4 :List(Text);
  missing @5 :List(Text);
  authored @6 :Bool;
  defined @7 :Bool;
  observed @8 :UInt32;
}

# Parallel identities/categories/transforms declare required conjunction legs.
# Transforms explicitly select magnitude, positive or negative standardized departures.
# Dense values contain strengths only for authored categories, in vocabulary order.
# Descriptive probabilities retain the LEGACY symmetric prior only among defined
# categories; unknown categories cannot contribute a prior to the feature vector.
interface Category {
  write @0 (cut :Record, identities :List(Text), categories :List(Text), vocabulary :List(Text), transforms :List(Text)) -> stream;
  done @1 () -> CategoryResult;
}

struct CategoryResult {
  union {
    idle @0 :Void;
    ready :group {
      reading @1 :CategoryReading;
      values @2 :List(Float64);
      support @3 :List(Float64);
      labels @4 :List(Text);
      present @5 :List(Bool);
    }
  }
}
