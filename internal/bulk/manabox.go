package bulk

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

type manaBoxRow struct {
	LineNo          int
	BinderName      string
	BinderType      string
	CardName        string
	SetCode         string
	ScryfallID      string
	CollectorNumber string
	Foil            bool
	Quantity        int
}

var manaBoxColumns = []string{
	"Binder Name", "Binder Type", "Name", "Set code",
	"Scryfall ID", "Collector number", "Foil", "Quantity",
}

func parseManaBoxCSV(r io.Reader) ([]manaBoxRow, error) {
	reader := csv.NewReader(r)

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: reading header: %v", ErrInvalidFile, err)
	}

	col := indexHeader(header)
	if err := requireColumns(col, manaBoxColumns...); err != nil {
		return nil, err
	}

	var rows []manaBoxRow
	line := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", ErrInvalidFile, line, err)
		}

		quantity, err := strconv.Atoi(record[col["Quantity"]])
		if err != nil || quantity < 1 {
			return nil, fmt.Errorf("%w: line %d: invalid Quantity %q", ErrInvalidFile, line, record[col["Quantity"]])
		}

		rows = append(rows, manaBoxRow{
			LineNo:          line,
			BinderName:      record[col["Binder Name"]],
			BinderType:      record[col["Binder Type"]],
			CardName:        record[col["Name"]],
			SetCode:         record[col["Set code"]],
			ScryfallID:      record[col["Scryfall ID"]],
			CollectorNumber: record[col["Collector number"]],
			Foil:            record[col["Foil"]] == "foil",
			Quantity:        quantity,
		})
	}

	return rows, nil
}
