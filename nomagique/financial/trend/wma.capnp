@0xcb44823af6c831bd;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Wma {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
