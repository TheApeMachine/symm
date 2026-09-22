@0x89cbf9cc77dce775;
using Go = import "/go.capnp";
$Go.package("projectionfixture");
$Go.import("github.com/theapemachine/symm/nomagique/compiler/testdata/projectionfixture");

# Wire fixtures exercise the same generated accessors and registry as real nodes.
struct Value {
 label @0 :Text = "default label";
 amount @1 :Float64 = 12.5;
 count @2 :UInt64 = 18446744073709551615;
 points @3 :List(Float64);
 children @4 :List(Value);
 groups @5 :List(List(Text));
 blob @6 :Data;
 union {
  missing @7 :Void;
  found :group {
   score @8 :Float32 = 2.5;
   visible @9 :Bool = true;
  }
 }
 child @10 :Value;
 signed @11 :Int64 = -9223372036854775808;
}

interface Output {
 write @0 () -> stream;
 done @1 () -> (value :Value, points :List(Float64));
}
