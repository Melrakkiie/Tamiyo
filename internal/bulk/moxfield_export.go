package bulk

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

var moxfieldExportHeader = []string{"Count", "Name", "Edition", "Foil", "Collector Number"}

type moxfieldGroupKey struct {
	Name            string
	SetCode         string
	CollectorNumber string
	Foil            bool
}

func (s *Service) ExportMoxfieldCollection(ctx context.Context, userID string, storageID *int, w io.Writer) error {
	cards, err := s.exportedCards(ctx, userID, storageID)
	if err != nil {
		return err
	}

	groups := make(map[moxfieldGroupKey]int)
	for _, c := range cards {
		groups[moxfieldGroupKey{
			Name:            c.Name,
			SetCode:         strings.ToLower(c.SetCode),
			CollectorNumber: c.CollectorNumber,
			Foil:            c.Foil,
		}]++
	}

	keys := make([]moxfieldGroupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}
		if keys[i].SetCode != keys[j].SetCode {
			return keys[i].SetCode < keys[j].SetCode
		}
		if keys[i].CollectorNumber != keys[j].CollectorNumber {
			return keys[i].CollectorNumber < keys[j].CollectorNumber
		}
		return !keys[i].Foil && keys[j].Foil
	})

	writer := csv.NewWriter(w)
	if err := writer.Write(moxfieldExportHeader); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}

	for _, k := range keys {
		foil := ""
		if k.Foil {
			foil = "foil"
		}
		record := []string{strconv.Itoa(groups[k]), k.Name, k.SetCode, foil, k.CollectorNumber}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("writing row: %w", err)
		}
	}

	writer.Flush()
	return writer.Error()
}
