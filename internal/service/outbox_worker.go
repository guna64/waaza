package service

import (
	"context"
	"log"
	"time"

	"github.com/guna64/waaza/internal/store"
)

func StartOutboxWorker(ctx context.Context, out *store.OutboxStore, svc *Service) {
	if out == nil || !out.Enabled() { return }
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				it, ok, err := out.ClaimOne()
				if err != nil {
					log.Printf("outbox claim error: %v", err)
					continue
				}
				if !ok { continue }
				waID, sendErr := svc.SendText(it.Payload.Phone, it.Payload.Message)
				if sendErr != nil {
					_ = out.MarkFailed(it.ID, it.Attempt+1, it.MaxAttempt, sendErr.Error())
					continue
				}
				_ = out.MarkSent(it.ID, waID)
			}
		}
	}()
}
