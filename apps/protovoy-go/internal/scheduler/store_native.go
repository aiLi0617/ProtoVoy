//go:build !js

package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	"protovoy.dev/protovoy/internal/project"
)

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS scheduled_jobs (id TEXT PRIMARY KEY, document BLOB NOT NULL)")
	if err == nil {
		_, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS scheduled_results (sequence INTEGER PRIMARY KEY AUTOINCREMENT, document BLOB NOT NULL)")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Create(ctx context.Context, name string, interval int, enabled bool, request project.Request, now time.Time) (Job, error) {
	if interval < 10 || interval > 31536000 || len(name) > 1024 {
		return Job{}, errors.New("schedule requires an interval from 10 to 31536000 seconds and a name within 1024 bytes")
	}
	data, err := project.CaptureHistoryRequest(request)
	if err != nil {
		return Job{}, err
	}
	job := Job{ID: rand.Text(), Name: name, Enabled: enabled, IntervalSeconds: interval,
		NextRunAt: now.UTC().Add(time.Duration(interval) * time.Second), CreatedAt: now.UTC(), Request: data}
	document, err := json.Marshal(job)
	if err != nil {
		return Job{}, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO scheduled_jobs (id, document) VALUES (?, ?)", job.ID, document)
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) List(ctx context.Context) ([]Job, error) {
	return listJobs(ctx, s.db)
}

type jobReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listJobs(ctx context.Context, reader jobReader) ([]Job, error) {
	rows, err := reader.QueryContext(ctx, "SELECT document FROM scheduled_jobs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		var data []byte
		var job Job
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &job) != nil || job.ID == "" || job.IntervalSeconds < 10 ||
			job.IntervalSeconds > 31536000 || job.NextRunAt.IsZero() {
			return nil, errors.New("invalid persisted schedule")
		}
		if _, err := project.OpenHistoryRequest(job.Request); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ClaimDue advances deadlines durably before returning work to the executor.
func (s *Store) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Job, error) {
	return s.claimDue(ctx, now, limit, nil)
}

func (s *Store) claimDue(ctx context.Context, now time.Time, limit int, active map[string]bool) ([]Job, error) {
	if limit < 1 || limit > 16 {
		return nil, errors.New("schedule claim limit must be from 1 to 16")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	jobs, err := listJobs(ctx, tx)
	if err != nil {
		return nil, err
	}
	due := []Job{}
	for _, job := range jobs {
		if !job.Enabled || active[job.ID] || job.NextRunAt.After(now) || len(due) == limit {
			continue
		}
		job.NextRunAt = now.UTC().Add(time.Duration(job.IntervalSeconds) * time.Second)
		data, err := json.Marshal(job)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE scheduled_jobs SET document = ? WHERE id = ?", data, job.ID); err != nil {
			return nil, err
		}
		due = append(due, job)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return due, nil
}

func (s *Store) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM scheduled_jobs WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count != 0, err
}

func (s *Store) SetEnabled(ctx context.Context, id string, enabled bool, now time.Time) (Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	var data []byte
	if err := tx.QueryRowContext(ctx, "SELECT document FROM scheduled_jobs WHERE id = ?", id).Scan(&data); err != nil {
		return Job{}, err
	}
	var job Job
	if json.Unmarshal(data, &job) != nil || job.IntervalSeconds < 10 || job.IntervalSeconds > 31536000 {
		return Job{}, errors.New("invalid persisted schedule")
	}
	if enabled && !job.Enabled {
		job.NextRunAt = now.UTC().Add(time.Duration(job.IntervalSeconds) * time.Second)
	}
	job.Enabled = enabled
	data, err = json.Marshal(job)
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE scheduled_jobs SET document = ? WHERE id = ?", data, id); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}
