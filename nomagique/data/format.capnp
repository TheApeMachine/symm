using Go = import "/go.capnp";
@0x8f51d0267584af93;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Format writes template with each {n} replaced by the raw bytes of values[n],
# so literal text and computed bytes (a digest, a token) compose into one
# message. {{ and }} are literal braces. Values gather, so Format is asked on
# every evaluation: when none arrived it is idle; when some arrived, a
# placeholder naming one that did not is an error, never an empty substitution.
interface Format {
  write @0 (template :Text, values :List(Data)) -> stream;
  done @1 () -> Formatted;
}

struct Formatted {
  union { idle @0 :Void; out @1 :Data; }
}
