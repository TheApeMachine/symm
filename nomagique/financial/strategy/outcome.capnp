@0xa1ce1be51ccefcc3;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface Outcome {
    write @0 (
        value :Float64,
        action :Int64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
