package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"sync"
)

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

type UserStore struct {
	mu    sync.RWMutex
	users map[string]User
	db    *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{users: map[string]User{}, db: db}
}

func (s *UserStore) List() []User {
	if s.db != nil {
		rows, err := s.db.Query(`SELECT id,name,token FROM waaza_users ORDER BY name ASC`)
		if err == nil {
			defer rows.Close()
			out := []User{}
			for rows.Next() {
				var u User
				if rows.Scan(&u.ID, &u.Name, &u.Token) == nil {
					out = append(out, u)
				}
			}
			return out
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out
}

func (s *UserStore) Create(name, token string) User {
	u := User{ID: randUserID(), Name: name, Token: token}
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO waaza_users (id,name,token) VALUES ($1,$2,$3)`, u.ID, u.Name, u.Token)
		return u
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	return u
}

func (s *UserStore) Delete(id string) bool {
	if s.db != nil {
		res, err := s.db.Exec(`DELETE FROM waaza_users WHERE id=$1`, id)
		if err != nil {
			return false
		}
		n, _ := res.RowsAffected()
		return n > 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return false
	}
	delete(s.users, id)
	return true
}

func randUserID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
