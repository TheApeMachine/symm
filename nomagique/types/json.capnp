using Go = import "/go.capnp";
@0xe88f3215f44a4ecd;
$Go.package("types");
$Go.import("github.com/theapemachine/symm/nomagique/types");

interface JSON {
  write @0 (text :Text) -> stream;
  done @1 () -> (out :Data);
}
