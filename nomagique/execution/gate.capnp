using Go = import "/go.capnp";
@0xffb921aa3b427c35;
$Go.package("execution");
$Go.import("nomagique/execution");

interface Gate {
  write @0 (text :Text) -> stream;
  done @1 () -> (out :Text);
}
