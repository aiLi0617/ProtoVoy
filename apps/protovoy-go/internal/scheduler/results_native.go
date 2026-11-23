//go:build !js

package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) Record(ctx context.Context, jobID string, started time.Time, duration time.Duration, success bool, status int) (Execution, error) {
	if jobID == "" || started.IsZero() || duration < 0 || status < 0 || status > 999 {
		return Execution{}, errors.New("invalid scheduled execution summary")
	}
	result := Execution{ID: rand.Text(), JobID: jobID, StartedAt: started.UTC(), DurationMS: duration.Milliseconds(), Success: success, Status: status}
	data, err := json.Marshal(result)
	if err != nil {
		return Execution{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Execution{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO scheduled_results (document) VALUES (?)", data); err != nil {
		return Execution{}, err
	}
	var document []byte
	err = tx.QueryRowContext(ctx, "SELECT document FROM scheduled_jobs WHERE id = ?", jobID).Scan(&document)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Execution{}, err
	}
	if err == nil {
		var job Job
		if json.Unmarshal(document, &job) != nil {
			return Execution{}, errors.New("invalid persisted schedule")
		}
		job.LastExecutionID = result.ID
		document, err = json.Marshal(job)
		if err != nil {
			return Execution{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE scheduled_jobs SET document = ? WHERE id = ?", document, jobID); err != nil {
			return Execution{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM scheduled_results WHERE sequence NOT IN (SELECT sequence FROM scheduled_results ORDER BY sequence DESC LIMIT 500)"); err != nil {
		return Execution{}, err
	}
	if err := tx.Commit(); err != nil {
		return Execution{}, err
	}
	return result, nil
}

func (s *Store) Executions(ctx context.Context) ([]Execution, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT document FROM scheduled_results ORDER BY sequence DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []Execution{}
	for rows.Next() {
		var data []byte
		var result Execution
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &result) != nil {
			return nil, errors.New("invalid execution summary")
		}
		results = append(results, result)
	}
	return results, rows.Err()
}
