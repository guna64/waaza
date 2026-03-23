package wa

type Status struct {
	Connected bool `json:"connected"`
	LoggedIn  bool `json:"logged_in"`
}

type Client interface {
	Connect() error
	Disconnect() error
	Logout() error
	Status() Status
	QR() string
	SendText(phone, message string) (string, error)
}
