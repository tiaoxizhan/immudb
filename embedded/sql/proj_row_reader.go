/*
Copyright 2021 CodeNotary, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package sql

import "fmt"

type projectedRowReader struct {
	e *Engine

	rowReader RowReader

	tableAlias string

	selectors []Selector

	limit uint64

	read uint64
}

func (e *Engine) newProjectedRowReader(rowReader RowReader, tableAlias string, selectors []Selector, limit uint64) (*projectedRowReader, error) {
	return &projectedRowReader{
		e:          e,
		rowReader:  rowReader,
		tableAlias: tableAlias,
		selectors:  selectors,
		limit:      limit,
	}, nil
}

func (pr *projectedRowReader) ImplicitDB() string {
	return pr.rowReader.ImplicitDB()
}

func (pr *projectedRowReader) ImplicitTable() string {
	if pr.tableAlias == "" {
		return pr.rowReader.ImplicitTable()
	}

	return pr.tableAlias
}

func (pr *projectedRowReader) Columns() ([]*ColDescriptor, error) {
	colsBySel, err := pr.colsBySelector()
	if err != nil {
		return nil, err
	}

	colsByPos := make([]*ColDescriptor, len(pr.selectors))

	for i, sel := range pr.selectors {
		aggFn, db, table, col := sel.resolve(pr.rowReader.ImplicitDB(), pr.rowReader.ImplicitTable())

		if pr.tableAlias != "" {
			db = pr.ImplicitDB()
			table = pr.tableAlias
		}

		if aggFn == "" && sel.alias() != "" {
			col = sel.alias()
		}

		if aggFn != "" {
			aggFn = ""
			col = sel.alias()
			if col == "" {
				col = fmt.Sprintf("col%d", i)
			}
		}

		encSel := EncodeSelector(aggFn, db, table, col)
		colsByPos[i] = &ColDescriptor{Selector: encSel, Type: colsBySel[encSel]}
	}

	return colsByPos, nil
}

func (pr *projectedRowReader) colsBySelector() (map[string]SQLValueType, error) {
	colDescriptors := make(map[string]SQLValueType, len(pr.selectors))

	dsColDescriptors, err := pr.rowReader.colsBySelector()
	if err != nil {
		return nil, err
	}

	for i, sel := range pr.selectors {
		aggFn, db, table, col := sel.resolve(pr.rowReader.ImplicitDB(), pr.rowReader.ImplicitTable())

		encSel := EncodeSelector(aggFn, db, table, col)

		colDesc, ok := dsColDescriptors[encSel]
		if !ok {
			return nil, ErrColumnDoesNotExist
		}

		if pr.tableAlias != "" {
			db = pr.ImplicitDB()
			table = pr.tableAlias
		}

		if aggFn == "" && sel.alias() != "" {
			col = sel.alias()
		}

		if aggFn != "" {
			aggFn = ""
			col = sel.alias()
			if col == "" {
				col = fmt.Sprintf("col%d", i)
			}
		}

		colDescriptors[EncodeSelector(aggFn, db, table, col)] = colDesc
	}

	return colDescriptors, nil
}

func (pr *projectedRowReader) Read() (*Row, error) {
	if pr.limit > 0 && pr.read == pr.limit {
		return nil, ErrNoMoreRows
	}

	row, err := pr.rowReader.Read()
	if err != nil {
		return nil, err
	}

	prow := &Row{
		Values: make(map[string]TypedValue, len(pr.selectors)),
	}

	for i, sel := range pr.selectors {
		aggFn, db, table, col := sel.resolve(pr.rowReader.ImplicitDB(), pr.rowReader.ImplicitTable())

		encSel := EncodeSelector(aggFn, db, table, col)

		val, ok := row.Values[encSel]
		if !ok {
			return nil, ErrColumnDoesNotExist
		}

		if pr.tableAlias != "" {
			db = pr.ImplicitDB()
			table = pr.tableAlias
		}

		if aggFn == "" && sel.alias() != "" {
			col = sel.alias()
		}

		if aggFn != "" {
			aggFn = ""
			col = sel.alias()
			if col == "" {
				col = fmt.Sprintf("col%d", i)
			}
		}

		prow.Values[EncodeSelector(aggFn, db, table, col)] = val
	}

	pr.read++

	return prow, nil
}

func (pr *projectedRowReader) Close() error {
	return pr.rowReader.Close()
}
