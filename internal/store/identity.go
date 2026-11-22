package store

import "context"

func (s *Store) SetNodeName(ctx context.Context, name string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE node_identity SET name=? WHERE singleton=1", name)
	return err
}
