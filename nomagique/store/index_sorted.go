package store

import (
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

// sorted admits a complete, pinned archive snapshot before publishing it.
// Physical Parquet file order is not sequence order. The existing Index owns
// the same exact coordinates in both live lookup and cold sequential modes.
func (server *IndexServer) sorted(args Index_write_Params, appends capnp.DataList, partitions, order []string) error {
	requests, err := args.Request()

	if err != nil {
		return server.Error(errnie.Err(errnie.Validation, "index: read requests", err))
	}

	if requests.Len() != 0 {
		return server.Error(errnie.Err(errnie.Validation, "index: sequential archive mode does not accept random-access requests", nil))
	}

	server.unordered = true
	server.row = nil

	if err := arrived(appends, func(document []byte) error {
		if server.sealed {
			return errnie.Err(errnie.Validation, "index: rows arrived after the archive was sealed", nil)
		}

		return server.append(document, partitions, order)
	}); err != nil {
		return server.Error(err)
	}

	if args.Exhausted() && !server.sealed {
		if err := server.sealIndex(); err != nil {
			return err
		}
	}

	if len(server.ordered) == 0 {
		return nil
	}

	server.row = server.ordered[0].document
	server.ordered[0] = indexed{}
	server.ordered = server.ordered[1:]
	return nil
}

func (server *IndexServer) sealIndex() error {
	names := make([]string, 0, len(server.partitions))

	for name := range server.partitions {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		rows := server.partitions[name]
		slices.SortFunc(rows, func(left, right indexed) int { return compare(left.order, right.order) })

		for index := 1; index < len(rows); index++ {
			if compare(rows[index-1].order, rows[index].order) == 0 {
				return server.Error(errnie.Err(errnie.Conflict, "index: duplicate archived coordinate in partition "+name, nil))
			}
		}

		server.ordered = append(server.ordered, rows...)
	}

	clear(server.partitions)
	server.sealed = true
	return nil
}
