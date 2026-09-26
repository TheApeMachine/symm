package http

import (
	"bytes"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"strconv"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/store/tables"
)

/* inspectionRoute declares the HTTP projection of an authored analytical query. */
type inspectionRoute struct {
	SQL       string `json:"sql"`
	Parameter string `json:"parameter,omitempty"`
	Shape     string `json:"shape"`
}

/* inspection owns the HTTP node's retained query capability and route declarations. */
type inspection struct {
	sync.RWMutex
	query       tables.Query
	routes      map[string]inspectionRoute
	declaration string
}

/* configure retains the graph's query capability without opening its database. */
func (inspection *inspection) configure(query tables.Query, routes string) error {
	inspection.Lock()
	defer inspection.Unlock()

	if routes != "" && routes != inspection.declaration {
		var declared map[string]inspectionRoute

		if err := json.Unmarshal([]byte(routes), &declared); err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "http: inspection routes", err))
		}
		for path, route := range declared {
			if route.SQL == "" || (route.Shape != "rows" && route.Shape != "symbols" && route.Shape != "object") {
				return errnie.Error(errnie.Err(errnie.Validation, "http: invalid query route: "+path, nil))
			}
		}
		inspection.routes = declared
		inspection.declaration = routes
	}

	if query.IsValid() && !query.IsSame(inspection.query) {
		retained := query.AddRef()
		inspection.query.Release()
		inspection.query = retained
	}
	return nil
}

/* Close releases the HTTP node's reference to the analytical node. */
func (inspection *inspection) Close() {
	inspection.Lock()
	defer inspection.Unlock()
	inspection.query.Release()
	inspection.query = tables.Query{}
}

/* ServeHTTP translates HTTP requests into atomic Cap'n Proto Query calls. */
func (inspection *inspection) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	inspection.RLock()
	query := inspection.query

	if query.IsValid() {
		query = query.AddRef()
	}
	route, exists := inspection.routes[request.URL.Path]
	inspection.RUnlock()

	if !query.IsValid() {
		stdhttp.Error(writer, "query node is not connected", stdhttp.StatusServiceUnavailable)
		return
	}
	defer query.Release()
	statement := route.SQL
	arrow := request.URL.Path == "/workbench/query"

	if !arrow && !exists {
		stdhttp.Error(writer, "query route is not configured", stdhttp.StatusServiceUnavailable)
		return
	}

	if arrow {
		var input struct {
			SQL string `json:"sql"`
		}
		decoder := json.NewDecoder(request.Body)
		if err := decoder.Decode(&input); err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusBadRequest)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			stdhttp.Error(writer, "expected one SQL request", stdhttp.StatusBadRequest)
			return
		}
		statement = input.SQL
	}
	parameter := ""

	if route.Parameter != "" {
		parameter = request.URL.Query().Get(route.Parameter)
		if parameter == "" && route.Parameter == "run" {
			parameter = request.URL.Query().Get("epoch")
		}
		if _, err := strconv.ParseInt(parameter, 10, 64); err != nil {
			stdhttp.Error(writer, "a valid run epoch is required", stdhttp.StatusBadRequest)
			return
		}
	}
	future, release := query.Query(request.Context(), func(params tables.Query_query_Params) error {
		params.SetJson(!arrow)
		if err := params.SetSql(statement); err != nil {
			return err
		}
		if route.Parameter == "" {
			return nil
		}
		parameters, err := params.NewParameters(1)
		if err != nil {
			return err
		}
		return parameters.Set(0, parameter)
	})
	defer release()
	result, err := future.Struct()

	if err != nil {
		stdhttp.Error(writer, err.Error(), stdhttp.StatusInternalServerError)
		return
	}
	output, err := result.Out()

	if err != nil {
		stdhttp.Error(writer, err.Error(), stdhttp.StatusInternalServerError)
		return
	}

	if !arrow {
		output, err = route.project(output)
		if err != nil {
			stdhttp.Error(writer, err.Error(), stdhttp.StatusInternalServerError)
			return
		}
	}
	writer.Header().Set("Content-Type", "application/json")

	if arrow {
		writer.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
	}

	if _, err := writer.Write(output); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "http: query response", err))
	}
}

/* project applies only the endpoint's declared outer JSON shape; values stay typed. */
func (route inspectionRoute) project(input []byte) ([]byte, error) {
	if route.Shape == "rows" {
		return input, nil
	}

	if route.Shape == "symbols" {
		var rows []struct {
			Symbol string `json:"symbol"`
		}
		if err := json.Unmarshal(input, &rows); err != nil {
			return nil, errnie.Error(err)
		}
		symbols := make([]string, len(rows))
		for index, row := range rows {
			symbols[index] = row.Symbol
		}
		output, err := json.Marshal(symbols)
		return output, errnie.Error(err)
	}
	var rows []json.RawMessage

	if err := json.Unmarshal(input, &rows); err != nil {
		return nil, errnie.Error(err)
	}

	if len(rows) != 1 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "http: object query must return exactly one row", nil))
	}
	return bytes.Clone(rows[0]), nil
}
