@0xbf89e924ceb02271;

using Go = import "/go.capnp";
$Go.package("compiler");
$Go.import("github.com/theapemachine/symm/nomagique/compiler");

interface TestUnion {
  struct Result {
    union {
      yes @0 :Float64;
      no @1 :Void;
    }
  }

  write @0 (in :Float64) -> stream;
  done @1 () -> Result;
}

interface TestSinkVoid {
  write @0 (in :Void) -> stream;
  done @1 () -> (ok :Bool);
}
