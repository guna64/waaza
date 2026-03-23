package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

type ProxyConfig struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxy_url"`
}

type S3Config struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"access_key"`
	PathStyle     bool   `json:"path_style"`
	PublicURL     string `json:"public_url"`
	MediaDelivery string `json:"media_delivery"`
	RetentionDays int    `json:"retention_days"`
}

type Instance struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Token     string      `json:"token"`
	Webhook   string      `json:"webhook"`
	Events    []string    `json:"events"`
	History   int         `json:"history"`
	HMACKey   string      `json:"hmac_key,omitempty"`
	Proxy     ProxyConfig `json:"proxy_config"`
	S3        S3Config    `json:"s3_config"`
	Connected bool        `json:"connected"`
	LoggedIn  bool        `json:"logged_in"`
	JID       string      `json:"jid"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type InstanceStore struct {
	mu        sync.RWMutex
	instances map[string]Instance
	db        *sql.DB
}

func NewInstanceStore(db *sql.DB) *InstanceStore {
	return &InstanceStore{instances: map[string]Instance{}, db: db}
}

func (s *InstanceStore) List() []Instance {
	if s.db != nil {
		rows, err := s.db.Query(`SELECT id,name,token,webhook,events,history,hmac_key,proxy,s3,connected,logged_in,jid,created_at,updated_at FROM waaza_instances ORDER BY created_at DESC`)
		if err == nil {
			defer rows.Close()
			out := []Instance{}
			for rows.Next() {
				if in, ok := scanInstance(rows); ok {
					out = append(out, in)
				}
			}
			return out
		}
	}
	s.mu.RLock(); defer s.mu.RUnlock()
	out := make([]Instance, 0, len(s.instances))
	for _, i := range s.instances { out = append(out, i) }
	return out
}

func (s *InstanceStore) Create(in Instance) Instance {
	now := time.Now().UTC()
	in.ID = randID()
	if len(in.Events) == 0 { in.Events = []string{"All"} }
	in.CreatedAt = now
	in.UpdatedAt = now
	if s.db != nil {
		e, _ := json.Marshal(in.Events)
		p, _ := json.Marshal(in.Proxy)
		s3, _ := json.Marshal(in.S3)
		_, _ = s.db.Exec(`INSERT INTO waaza_instances (id,name,token,webhook,events,history,hmac_key,proxy,s3,connected,logged_in,jid,created_at,updated_at) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8::jsonb,$9::jsonb,$10,$11,$12,$13,$14)`,
			in.ID, in.Name, in.Token, in.Webhook, string(e), in.History, in.HMACKey, string(p), string(s3), in.Connected, in.LoggedIn, in.JID, in.CreatedAt, in.UpdatedAt)
		return in
	}
	s.mu.Lock(); defer s.mu.Unlock(); s.instances[in.ID] = in
	return in
}

func (s *InstanceStore) Get(id string) (Instance, bool) {
	if s.db != nil {
		row := s.db.QueryRow(`SELECT id,name,token,webhook,events,history,hmac_key,proxy,s3,connected,logged_in,jid,created_at,updated_at FROM waaza_instances WHERE id=$1`, id)
		in, ok := scanInstance(row)
		return in, ok
	}
	s.mu.RLock(); defer s.mu.RUnlock(); v, ok := s.instances[id]; return v, ok
}

func (s *InstanceStore) Delete(id string) bool {
	if s.db != nil {
		res, err := s.db.Exec(`DELETE FROM waaza_instances WHERE id=$1`, id)
		if err != nil { return false }
		n, _ := res.RowsAffected()
		return n > 0
	}
	s.mu.Lock(); defer s.mu.Unlock(); if _, ok := s.instances[id]; !ok { return false }; delete(s.instances, id); return true
}

func (s *InstanceStore) Update(id string, fn func(*Instance)) (Instance, bool) {
	if s.db != nil {
		in, ok := s.Get(id)
		if !ok { return Instance{}, false }
		fn(&in)
		in.UpdatedAt = time.Now().UTC()
		e, _ := json.Marshal(in.Events)
		p, _ := json.Marshal(in.Proxy)
		s3, _ := json.Marshal(in.S3)
		_, err := s.db.Exec(`UPDATE waaza_instances SET name=$2,token=$3,webhook=$4,events=$5::jsonb,history=$6,hmac_key=$7,proxy=$8::jsonb,s3=$9::jsonb,connected=$10,logged_in=$11,jid=$12,updated_at=$13 WHERE id=$1`,
			in.ID, in.Name, in.Token, in.Webhook, string(e), in.History, in.HMACKey, string(p), string(s3), in.Connected, in.LoggedIn, in.JID, in.UpdatedAt)
		if err != nil { return Instance{}, false }
		return in, true
	}
	s.mu.Lock(); defer s.mu.Unlock()
	v, ok := s.instances[id]; if !ok { return Instance{}, false }
	fn(&v); v.UpdatedAt = time.Now().UTC(); s.instances[id]=v
	return v, true
}

type scanner interface { Scan(dest ...any) error }

func scanInstance(sc scanner) (Instance, bool) {
	var in Instance
	var eventsB, proxyB, s3B []byte
	if err := sc.Scan(&in.ID,&in.Name,&in.Token,&in.Webhook,&eventsB,&in.History,&in.HMACKey,&proxyB,&s3B,&in.Connected,&in.LoggedIn,&in.JID,&in.CreatedAt,&in.UpdatedAt); err != nil {
		return Instance{}, false
	}
	_ = json.Unmarshal(eventsB, &in.Events)
	_ = json.Unmarshal(proxyB, &in.Proxy)
	_ = json.Unmarshal(s3B, &in.S3)
	if in.Events == nil { in.Events = []string{} }
	return in, true
}

func randID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
