package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxProcessing OutboxStatus = "processing"
	OutboxSent       OutboxStatus = "sent"
	OutboxFailed     OutboxStatus = "failed"
	OutboxDead       OutboxStatus = "dead"
)

type OutboxPayload struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

type OutboxItem struct {
	ID          string      `json:"id"`
	InstanceID  string      `json:"instance_id"`
	Payload     OutboxPayload `json:"payload"`
	Status      OutboxStatus `json:"status"`
	Attempt     int         `json:"attempt"`
	MaxAttempt  int         `json:"max_attempt"`
	NextRetryAt time.Time   `json:"next_retry_at"`
	LastError   string      `json:"last_error,omitempty"`
	WAMessageID string      `json:"wa_message_id,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type OutboxStore struct { db *sql.DB }

func NewOutboxStore(db *sql.DB) *OutboxStore { return &OutboxStore{db: db} }

func (s *OutboxStore) Enabled() bool { return s != nil && s.db != nil }

func (s *OutboxStore) Enqueue(instanceID string, p OutboxPayload, maxAttempt int) (string, error) {
	id := randID()
	if maxAttempt <= 0 { maxAttempt = 5 }
	b, _ := json.Marshal(p)
	_, err := s.db.Exec(`INSERT INTO waaza_outbox (id,instance_id,payload,status,attempt,max_attempt,next_retry_at,created_at,updated_at) VALUES ($1,$2,$3::jsonb,'pending',0,$4,NOW(),NOW(),NOW())`, id, instanceID, string(b), maxAttempt)
	return id, err
}

func (s *OutboxStore) Get(id string) (OutboxItem, bool) {
	row := s.db.QueryRow(`SELECT id,instance_id,payload,status,attempt,max_attempt,next_retry_at,last_error,wa_message_id,created_at,updated_at FROM waaza_outbox WHERE id=$1`, id)
	var it OutboxItem
	var p []byte
	if err := row.Scan(&it.ID,&it.InstanceID,&p,&it.Status,&it.Attempt,&it.MaxAttempt,&it.NextRetryAt,&it.LastError,&it.WAMessageID,&it.CreatedAt,&it.UpdatedAt); err != nil {
		return OutboxItem{}, false
	}
	_ = json.Unmarshal(p, &it.Payload)
	return it, true
}

func (s *OutboxStore) ClaimOne() (OutboxItem, bool, error) {
	tx, err := s.db.Begin()
	if err != nil { return OutboxItem{}, false, err }
	defer tx.Rollback()
	row := tx.QueryRow(`
		WITH cte AS (
			SELECT id FROM waaza_outbox
			WHERE status IN ('pending','failed') AND next_retry_at <= NOW()
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE waaza_outbox o
		SET status='processing', updated_at=NOW()
		FROM cte
		WHERE o.id = cte.id
		RETURNING o.id,o.instance_id,o.payload,o.status,o.attempt,o.max_attempt,o.next_retry_at,o.last_error,o.wa_message_id,o.created_at,o.updated_at
	`)
	var it OutboxItem
	var p []byte
	if err := row.Scan(&it.ID,&it.InstanceID,&p,&it.Status,&it.Attempt,&it.MaxAttempt,&it.NextRetryAt,&it.LastError,&it.WAMessageID,&it.CreatedAt,&it.UpdatedAt); err != nil {
		if err == sql.ErrNoRows { return OutboxItem{}, false, nil }
		return OutboxItem{}, false, err
	}
	_ = json.Unmarshal(p, &it.Payload)
	if err := tx.Commit(); err != nil { return OutboxItem{}, false, err }
	return it, true, nil
}

func (s *OutboxStore) MarkSent(id, waID string) error {
	_, err := s.db.Exec(`UPDATE waaza_outbox SET status='sent', wa_message_id=$2, updated_at=NOW() WHERE id=$1`, id, waID)
	return err
}

func (s *OutboxStore) MarkFailed(id string, attempt, maxAttempt int, lastErr string) error {
	status := "failed"
	if attempt >= maxAttempt { status = "dead" }
	backoff := time.Duration(1<<min(attempt, 6)) * time.Second
	_, err := s.db.Exec(`UPDATE waaza_outbox SET status=$2, attempt=$3, last_error=$4, next_retry_at=$5, updated_at=NOW() WHERE id=$1`, id, status, attempt, lastErr, time.Now().Add(backoff))
	return err
}

func min(a,b int) int { if a<b { return a }; return b }
