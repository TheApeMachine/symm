@0x91013590344b2a6b;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface And {
    write @0 (
        action1 :Int64,
        action2 :Int64,
    ) -> stream;

    done @1 () -> (
        action :Int64,
    );
}
