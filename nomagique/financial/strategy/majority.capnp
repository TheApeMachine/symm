@0xb6e116ac4b814fb3;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface Majority {
    write @0 (
        action1 :Int64,
        action2 :Int64,
        action3 :Int64,
    ) -> stream;

    done @1 () -> (
        action :Int64,
    );
}
