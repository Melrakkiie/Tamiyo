package bulk

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
)

var manaBoxExportHeader = []string{
	"Binder Name", "Binder Type", "Name", "Set code",
	"Scryfall ID", "Collector number", "Foil", "Quantity",
}

type cardGroupKey struct {
	Name            string
	SetCode         string
	ScryfallID      string
	CollectorNumber string
	Foil            bool
}

const unsortedStorageID = 0

func (s *Service) ExportManaBox(ctx context.Context, userID string, w io.Writer) error {
	cards, err := s.loadAllCards(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading cards: %w", err)
	}
	storagesByID, err := s.loadAllStoragesByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading storages: %w", err)
	}

	type bucketInfo struct {
		name  string
		typ   string
		order int
	}

	buckets := make(map[int]map[cardGroupKey]int)
	infoByBucket := make(map[int]bucketInfo)

	for _, c := range cards {
		bucketID := unsortedStorageID
		if c.StorageID != nil {
			bucketID = *c.StorageID
		}

		if buckets[bucketID] == nil {
			buckets[bucketID] = make(map[cardGroupKey]int)
		}
		buckets[bucketID][cardGroupKey{
			Name: c.Name, SetCode: c.SetCode, ScryfallID: c.ScryfallID,
			CollectorNumber: c.CollectorNumber, Foil: c.Foil,
		}]++

		if _, ok := infoByBucket[bucketID]; ok {
			continue
		}
		switch {
		case bucketID == unsortedStorageID:
			infoByBucket[bucketID] = bucketInfo{name: "Unsorted", typ: "binder", order: math.MaxInt32}
		case storagesByID[bucketID].ID != 0:
			st := storagesByID[bucketID]
			infoByBucket[bucketID] = bucketInfo{name: st.Name, typ: st.Type, order: bucketID}
		default:
			infoByBucket[bucketID] = bucketInfo{name: fmt.Sprintf("storage-%d", bucketID), typ: "binder", order: bucketID}
		}
	}

	bucketIDs := make([]int, 0, len(buckets))
	for id := range buckets {
		bucketIDs = append(bucketIDs, id)
	}
	sort.Slice(bucketIDs, func(i, j int) bool {
		return infoByBucket[bucketIDs[i]].order < infoByBucket[bucketIDs[j]].order
	})

	writer := csv.NewWriter(w)
	if err := writer.Write(manaBoxExportHeader); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}

	for _, bucketID := range bucketIDs {
		info := infoByBucket[bucketID]
		for _, key := range sortedGroupKeys(buckets[bucketID]) {
			foil := ""
			if key.Foil {
				foil = "foil"
			}
			record := []string{
				info.name, info.typ, key.Name, key.SetCode,
				key.ScryfallID, key.CollectorNumber, foil,
				strconv.Itoa(buckets[bucketID][key]),
			}
			if err := writer.Write(record); err != nil {
				return fmt.Errorf("writing row: %w", err)
			}
		}
	}

	writer.Flush()
	return writer.Error()
}

func sortedGroupKeys(groups map[cardGroupKey]int) []cardGroupKey {
	keys := make([]cardGroupKey, 0, len(groups))
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
	return keys
}
