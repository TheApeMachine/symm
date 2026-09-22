@0xa76bec5e52135f7c;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Hma {
    write @0 (
        value :Float64,
    ) -> stream;

    done @1 () -> (
        result :Float64,
    );
}
