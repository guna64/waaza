package wa

import (
	"fmt"
	"time"
)

type MockAdapter struct {
	connected bool
	loggedIn  bool
}

func NewMockAdapter() *MockAdapter { return &MockAdapter{} }

func (m *MockAdapter) Connect() error {
	m.connected = true
	if !m.loggedIn {
		// simulate first-time pairing path
		m.loggedIn = false
	}
	return nil
}

func (m *MockAdapter) Disconnect() error {
	m.connected = false
	return nil
}

func (m *MockAdapter) Logout() error {
	m.connected = false
	m.loggedIn = false
	return nil
}

func (m *MockAdapter) Status() Status {
	return Status{Connected: m.connected, LoggedIn: m.loggedIn}
}

func (m *MockAdapter) QR() string {
	if m.loggedIn {
		return ""
	}
	return "QR-PLACEHOLDER"
}

func (m *MockAdapter) SendText(phone, message string) (string, error) {
	if !m.connected {
		return "", fmt.Errorf("not connected")
	}
	_ = phone
	_ = message
	return fmt.Sprintf("msg_%d", time.Now().UnixNano()), nil
}
