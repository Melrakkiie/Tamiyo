package deck

import "context"

func (s *Service) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	return s.repo.CountCopiesByName(ctx, userID, nameKeys)
}
