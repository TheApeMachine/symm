using Go = import "/go.capnp";
@0x840dc7107531e4e3;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Gather retains each wired metric's latest observation. It stays idle until
# every coordinate has initialized, then emits the complete cut on each update.
# Families name consecutive ranges for readiness reporting; one active metric
# never makes its partially initialized family ready.
struct Gathered {
  readiness @3 :Data;
  phase     @4 :Text;
  epoch @5 :Int64;
  sequence @6 :Int64;
  epochs @7 :List(Int64);
  sequences @8 :List(Int64);
  scope @9 :Text;
  row @10 :Data;
  union {
    idle @0 :Void;
    gathered :group {
      values  @1 :List(Float64);
      present @2 :List(Bool);
    }
  }
}

interface Gather {
  write @0 (values :List(Float64), present :List(Bool), families :Text, epoch :Int64, sequence :Int64, scope :Text, identities :List(Text), row :Data, observation :Data, requireProvenance :Bool) -> stream;
  done @1 () -> Gathered;
}
