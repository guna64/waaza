package wa

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waTypes "go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type WhatsmeowAdapter struct {
	client *whatsmeow.Client

	mu      sync.RWMutex
	lastQR  string
	started bool
}

func NewWhatsmeowAdapter(dbDriver, dbDSN string) (*WhatsmeowAdapter, error) {
	ctx := context.Background()
	container, err := sqlstore.New(ctx, dbDriver, dbDSN, waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("init sqlstore: %w", err)
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get device store: %w", err)
	}
	client := whatsmeow.NewClient(deviceStore, waLog.Noop)
	return &WhatsmeowAdapter{client: client}, nil
}

func (w *WhatsmeowAdapter) Connect() error {
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}
	if !w.client.IsLoggedIn() {
		qrChan, err := w.client.GetQRChannel(context.Background())
		if err == nil {
			go func() {
				for evt := range qrChan {
					w.mu.Lock()
					if evt.Event == "code" {
						w.lastQR = evt.Code
					} else if evt.Event == "success" {
						w.lastQR = ""
					}
					w.mu.Unlock()
				}
			}()
		}
	}
	if err := w.client.Connect(); err != nil {
		return err
	}
	w.mu.Lock()
	w.started = true
	w.mu.Unlock()
	return nil
}

func (w *WhatsmeowAdapter) Disconnect() error {
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}
	w.client.Disconnect()
	return nil
}

func (w *WhatsmeowAdapter) Logout() error {
	if w.client == nil {
		return fmt.Errorf("client not initialized")
	}
	return w.client.Logout(context.Background())
}

func (w *WhatsmeowAdapter) Status() Status {
	if w.client == nil {
		return Status{}
	}
	return Status{Connected: w.client.IsConnected(), LoggedIn: w.client.IsLoggedIn()}
}

func (w *WhatsmeowAdapter) QR() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastQR
}

func normalizePhone(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "+")
	return p
}

func (w *WhatsmeowAdapter) SendText(phone, message string) (string, error) {
	if w.client == nil {
		return "", fmt.Errorf("client not initialized")
	}
	if !w.client.IsConnected() {
		return "", fmt.Errorf("not connected")
	}
	jid := waTypes.NewJID(normalizePhone(phone), waTypes.DefaultUserServer)
	resp, err := w.client.SendMessage(context.Background(), jid, &waProto.Message{
		Conversation: proto.String(message),
	}, whatsmeow.SendRequestExtra{ID: waTypes.MessageID(uuid.NewString())})
	if err != nil {
		return "", err
	}
	return string(resp.ID), nil
}
