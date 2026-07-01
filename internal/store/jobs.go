package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Job is a queued unit of work: process one inbound image.
type Job struct {
	ID              int64
	Provider        string
	ProviderGroupID string
	MessageID       string
	MediaRef        string
	Status          string
	Attempts        int
}

// EnqueueJob durably records a pending job and returns its id.
func (s *Store) EnqueueJob(provider, providerGroupID, messageID, mediaRef string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO jobs (provider, provider_group_id, message_id, media_ref, status)
		 VALUES (?, ?, ?, ?, 'pending')`,
		provider, providerGroupID, messageID, mediaRef,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueuing job: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// ClaimJob atomically moves the oldest pending job to 'processing' and returns
// it, or ErrNoJob if the queue is empty. Safe under the single-writer pool.
func (s *Store) ClaimJob() (*Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var j Job
	err = tx.QueryRow(
		`SELECT id, provider, provider_group_id, message_id, media_ref, status, attempts
		   FROM jobs WHERE status = 'pending' ORDER BY id LIMIT 1`).
		Scan(&j.ID, &j.Provider, &j.ProviderGroupID, &j.MessageID, &j.MediaRef, &j.Status, &j.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoJob
	}
	if err != nil {
		return nil, fmt.Errorf("claiming job: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE jobs SET status = 'processing', attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		j.ID); err != nil {
		return nil, fmt.Errorf("marking processing: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	j.Attempts++
	return &j, nil
}

// ErrNoJob signals an empty queue.
var ErrNoJob = errors.New("no pending job")

// CompleteJob marks a job done.
func (s *Store) CompleteJob(id int64) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET status = 'done', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// FailJob returns the job to pending for another attempt, or marks it 'dead'
// once attempts reach maxAttempts. Reports whether the job is now dead.
func (s *Store) FailJob(id int64, maxAttempts int) (dead bool, err error) {
	var attempts int
	if err := s.db.QueryRow(`SELECT attempts FROM jobs WHERE id = ?`, id).Scan(&attempts); err != nil {
		return false, fmt.Errorf("reading attempts: %w", err)
	}
	status := "pending"
	if attempts >= maxAttempts {
		status = "dead"
		dead = true
	}
	if _, err := s.db.Exec(
		`UPDATE jobs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id); err != nil {
		return dead, fmt.Errorf("failing job: %w", err)
	}
	return dead, nil
}

// PendingJobs counts jobs awaiting processing (queue-depth metric).
func (s *Store) PendingJobs() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM jobs WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// RequeueStuckJobs returns any 'processing' jobs to 'pending' at startup, so a
// crash mid-processing doesn't strand work.
func (s *Store) RequeueStuckJobs() (int64, error) {
	res, err := s.db.Exec(
		`UPDATE jobs SET status = 'pending' WHERE status = 'processing'`)
	if err != nil {
		return 0, fmt.Errorf("requeuing stuck jobs: %w", err)
	}
	return res.RowsAffected()
}
