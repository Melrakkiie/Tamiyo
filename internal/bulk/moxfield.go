package bulk

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type moxfieldCollectionRow struct {
	LineNo          int
	CardName        string
	SetCode         string
	CollectorNumber string
	Foil            bool
	Quantity        int
}

var moxfieldCollectionColumns = []string{
	"Count", "Name", "Edition", "Foil", "Collector Number",
}

func parseMoxfieldCollectionCSV(r io.Reader) ([]moxfieldCollectionRow, error) {
	reader := csv.NewReader(r)

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: reading header: %v", ErrInvalidFile, err)
	}

	col := indexHeader(header)
	if err := requireColumns(col, moxfieldCollectionColumns...); err != nil {
		return nil, err
	}

	var rows []moxfieldCollectionRow
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

		quantity, err := strconv.Atoi(record[col["Count"]])
		if err != nil || quantity < 1 {
			return nil, fmt.Errorf("%w: line %d: invalid Count %q", ErrInvalidFile, line, record[col["Count"]])
		}

		rows = append(rows, moxfieldCollectionRow{
			LineNo:          line,
			CardName:        record[col["Name"]],
			SetCode:         record[col["Edition"]],
			CollectorNumber: record[col["Collector Number"]],
			Foil:            record[col["Foil"]] == "foil",
			Quantity:        quantity,
		})
	}

	return rows, nil
}

type moxfieldDeckLine struct {
	LineNo          int
	Quantity        int
	CardName        string
	SetCode         string
	CollectorNumber string
	Foil            bool
}

var moxfieldDeckLineRE = regexp.MustCompile(`^(\d+)x?\s+(.+?)\s+\(([A-Za-z0-9]+)\)\s+(\S+?)(\s+\*[FE]\*)?$`)

var plainDeckLineRE = regexp.MustCompile(`^(\d+)x?\s+(\S.*?)(\s+\*[FE]\*)?$`)

func parseMoxfieldDeckList(r io.Reader) ([]moxfieldDeckLine, error) {
	scanner := bufio.NewScanner(r)

	var lines []moxfieldDeckLine
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}

		matches := moxfieldDeckLineRE.FindStringSubmatch(text)
		if matches == nil {
			plain := plainDeckLineRE.FindStringSubmatch(text)
			if plain == nil {
				return nil, fmt.Errorf("%w: line %d: unrecognized format %q", ErrInvalidFile, lineNo, text)
			}
			matches = []string{plain[0], plain[1], plain[2], "", "", plain[3]}
		}

		quantity, err := strconv.Atoi(matches[1])
		if err != nil || quantity < 1 {
			return nil, fmt.Errorf("%w: line %d: invalid quantity %q", ErrInvalidFile, lineNo, matches[1])
		}

		lines = append(lines, moxfieldDeckLine{
			LineNo:          lineNo,
			Quantity:        quantity,
			CardName:        matches[2],
			SetCode:         matches[3],
			CollectorNumber: matches[4],
			Foil:            matches[5] != "",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: file has no card lines", ErrInvalidFile)
	}

	return lines, nil
}
