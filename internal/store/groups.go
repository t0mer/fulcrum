package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Group is a WhatsApp group known to Fulcrum.
type Group struct {
	ID              int64      `json:"id"`
	ProviderGroupID string     `json:"provider_group_id"`
	Name            string     `json:"name"`
	Monitored       bool       `json:"monitored"`
	IsDestination   bool       `json:"is_destination"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
}

// UpsertGroup inserts or refreshes a group discovered from the provider,
// preserving the monitored/destination flags on conflict.
func (s *Store) UpsertGroup(providerGroupID, name string) error {
	_, err := s.db.Exec(
		`INSERT INTO groups (provider_group_id, name, last_seen)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(provider_group_id)
		 DO UPDATE SET name = excluded.name, last_seen = CURRENT_TIMESTAMP`,
		providerGroupID, name,
	)
	if err != nil {
		return fmt.Errorf("upserting group: %w", err)
	}
	return nil
}

// ListGroups returns all known groups ordered by name.
func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(
		`SELECT id, provider_group_id, name, monitored, is_destination, last_seen
		   FROM groups ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		var lastSeen sql.NullTime
		if err := rows.Scan(&g.ID, &g.ProviderGroupID, &g.Name, &g.Monitored, &g.IsDestination, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		if lastSeen.Valid {
			g.LastSeen = &lastSeen.Time
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetMonitored toggles whether a group's images are processed.
func (s *Store) SetMonitored(id int64, monitored bool) error {
	res, err := s.db.Exec(`UPDATE groups SET monitored = ? WHERE id = ?`, monitored, id)
	if err != nil {
		return fmt.Errorf("setting monitored: %w", err)
	}
	return affected(res)
}

// SetDestination makes exactly one group the forward target (or clears all when
// id is 0).
func (s *Store) SetDestination(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE groups SET is_destination = 0`); err != nil {
		return fmt.Errorf("clearing destination: %w", err)
	}
	if id != 0 {
		res, err := tx.Exec(`UPDATE groups SET is_destination = 1 WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("setting destination: %w", err)
		}
		if err := affected(res); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsMonitored reports whether inbound media from this provider group should be
// processed.
func (s *Store) IsMonitored(providerGroupID string) (bool, error) {
	var monitored bool
	err := s.db.QueryRow(
		`SELECT monitored FROM groups WHERE provider_group_id = ?`, providerGroupID).Scan(&monitored)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking monitored: %w", err)
	}
	return monitored, nil
}

// GroupName returns the display name for a provider group id, or the id itself
// if unknown.
func (s *Store) GroupName(providerGroupID string) string {
	var name string
	if err := s.db.QueryRow(
		`SELECT name FROM groups WHERE provider_group_id = ?`, providerGroupID).Scan(&name); err != nil {
		return providerGroupID
	}
	return name
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
