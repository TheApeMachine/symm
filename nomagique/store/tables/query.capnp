using Go = import "/go.capnp";
@0xcb5f7d81a4b120c9;
$Go.package("tables");
$Go.import("github.com/theapemachine/symm/nomagique/store/tables");

# One analytical session owns its catalog attachments and temporary views.
# Setup is graph-authored SQL, applied lazily on the first query, never on ingress.
# Graph SQL is single-flight: done polls without waiting. Changed parameters
# supersede an unfinished read; its rows never advance the new cursor.
interface Query {
 write @0 (setup :List(Text), sql :Text, parameters :List(Text)) -> stream;
 done @1 () -> (configured :Bool, out :Data, pending :Bool, superseded :UInt64);
 query @2 (sql :Text, json :Bool, parameters :List(Text)) -> (out :Data);
}
