@0xd5773c801fcc2ba5;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface DenormalizeActions {
    write @0 (
        action :Int64,
    ) -> stream;

    done @1 () -> (
        result :Int64,
    );
}
