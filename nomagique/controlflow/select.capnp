@0xd4d0b9f4ce4bbad1;
using Go = import "/go.capnp";
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

# Each branch admits zero or one nonempty arrival. Only the selected branch
# participates. No selected arrival produces absent, never the other branch.
interface Select {
  write @0 (test :Bool, yes :List(Data), no :List(Data)) -> stream;
  done @1 () -> Selection;
}
struct Selection { union { absent @0 :Void; out @1 :Data; } }
