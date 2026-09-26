using Go = import "/go.capnp";
@0xfa61802be979c3d4;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

interface Once {
  write @0 (trigger :Bool, through :Data, reset :Bool) -> stream;
  done @1 () -> OnceResult;
}

# An inactive branch must not schedule downstream side effects.
struct OnceResult {
 union {out @0 :Data; idle @2 :Void;}
 fired @1 :Bool;
}
