package service

import "github.com/guna64/waaza/internal/wa"

type Service struct {
	wa wa.Client
}

func New(w wa.Client) *Service { return &Service{wa: w} }

func (s *Service) Connect() error         { return s.wa.Connect() }
func (s *Service) Disconnect() error      { return s.wa.Disconnect() }
func (s *Service) Logout() error          { return s.wa.Logout() }
func (s *Service) Status() wa.Status      { return s.wa.Status() }
func (s *Service) QR() string             { return s.wa.QR() }
func (s *Service) SendText(p, m string) (string, error) { return s.wa.SendText(p, m) }
