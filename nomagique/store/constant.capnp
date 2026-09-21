using Go = import "/go.capnp";
@0xc826f6eb94916a2b;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

interface Constant {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
