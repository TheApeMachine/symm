using Go = import "/go.capnp";
@0xbe046c7490328bff;
$Go.package("crypto");
$Go.import("github.com/theapemachine/symm/nomagique/transport/crypto");

# Secret reads the process environment variable called name each time it is
# triggered, so credentials never live in a graph document. An unset or empty
# variable is an error, never an empty credential.
interface Secret {
  write @0 (trigger :Data, name :Text) -> stream;
  done @1 () -> (out :Data);
}
