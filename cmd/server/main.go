package main

import (
	"context"
	"database/sql"
	"log"

	"github.com/guna64/waaza/internal/api"
	"github.com/guna64/waaza/internal/config"
	"github.com/guna64/waaza/internal/service"
	"github.com/guna64/waaza/internal/store"
	"github.com/guna64/waaza/internal/wa"
)

func main() {
	cfg := config.Load()

	var adapter wa.Client
	if cfg.Provider == "whatsmeow" {
		realAdapter, err := wa.NewWhatsmeowAdapter(cfg.DBDriver, cfg.DBDSN)
		if err != nil {
			log.Fatalf("failed init whatsmeow adapter: %v", err)
		}
		adapter = realAdapter
		log.Printf("Waaza provider: whatsmeow")
	} else {
		adapter = wa.NewMockAdapter()
		log.Printf("Waaza provider: mock")
	}

	svc := service.New(adapter)

	var db = (*sql.DB)(nil)
	if cfg.DBDriver == "pgx" {
		dbOpened, err := store.OpenPostgres(cfg.DBDSN)
		if err != nil {
			log.Fatalf("failed init postgres store: %v", err)
		}
		db = dbOpened
	}
	users := store.NewUserStore(db)
	instances := store.NewInstanceStore(db)
	outbox := store.NewOutboxStore(db)
	service.StartOutboxWorker(context.Background(), outbox, svc)
	r := api.NewRouter(svc, cfg.APIKey, cfg.AdminToken, users, instances, outbox)

	log.Printf("Waaza listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
