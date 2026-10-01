package bulkimport

import "fmt"

func indexHeader(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	return index
}

func requireColumns(index map[string]int, required ...string) error {
	for _, name := range required {
		if _, ok := index[name]; !ok {
			return fmt.Errorf("%w: missing column %q", ErrInvalidFile, name)
		}
	}
	return nil
}
