@0xf07474f5b9d847d8;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface ActionsToAnnotations {
    write @0 (
        action :Int64,
    ) -> stream;

    done @1 () -> (
        annotation :Text,
    );
}
