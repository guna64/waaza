package api

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guna64/waaza/internal/middleware"
	"github.com/guna64/waaza/internal/service"
	"github.com/guna64/waaza/internal/store"
	qrcode "github.com/skip2/go-qrcode"
)

type sendTextReq struct {
	Phone   string `json:"phone" binding:"required"`
	Message string `json:"message" binding:"required"`
}

type authVerifyReq struct {
	Token string `json:"token" binding:"required"`
	Role  string `json:"role" binding:"required,oneof=user admin"`
}

type createUserReq struct {
	Name  string `json:"name" binding:"required"`
	Token string `json:"token" binding:"required"`
}

type createInstanceReq struct {
	Name      string   `json:"name" binding:"required"`
	Token     string   `json:"token" binding:"required"`
	Webhook   string   `json:"webhook"`
	Events    []string `json:"events"`
	History   int      `json:"history"`
	HMACKey   string   `json:"hmac_key"`
	ProxyURL  string   `json:"proxy_url"`
	ProxyOn   bool     `json:"proxy_enabled"`
	S3Enabled bool     `json:"s3_enabled"`
}

type updateWebhookReq struct {
	Webhook string   `json:"webhook" binding:"required"`
	Events  []string `json:"events"`
	Active  bool     `json:"active"`
}

func qrPayload(code string) gin.H {
	if strings.TrimSpace(code) == "" {
		return gin.H{"qrcode": "", "qrcode_image": ""}
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 256)
	if err != nil {
		return gin.H{"qrcode": code, "qrcode_image": ""}
	}
	return gin.H{
		"qrcode":       code,
		"qrcode_image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}
}

func NewRouter(svc *service.Service, userAPIKey, adminToken string, users *store.UserStore, instances *store.InstanceStore, outbox *store.OutboxStore) *gin.Engine {
	r := gin.Default()

	r.LoadHTMLGlob("web/templates/*.html")
	r.Static("/assets", "web/assets")

	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/dashboard") })
	r.GET("/dashboard", func(c *gin.Context) { c.HTML(http.StatusOK, "dashboard.html", gin.H{"title": "Waaza Dashboard"}) })
	r.GET("/dashboard/instance/:id", func(c *gin.Context) { c.HTML(http.StatusOK, "instance.html", gin.H{"title": "Waaza Instance"}) })

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"status": "ok"}})
	})

	r.GET("/api", func(c *gin.Context) { c.Redirect(http.StatusFound, "/api/") })
	r.GET("/api/", func(c *gin.Context) { c.HTML(http.StatusOK, "swagger.html", gin.H{"title": "Waaza API"}) })
	r.GET("/api/spec.yml", func(c *gin.Context) { c.File("openapi/spec.yml") })

	r.POST("/auth/verify", func(c *gin.Context) {
		var req authVerifyReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()})
			return
		}
		ok := false
		if req.Role == "admin" {
			ok = req.Token == adminToken
		} else {
			if req.Token == userAPIKey {
				ok = true
			} else {
				for _, inst := range instances.List() {
					if inst.Token == req.Token {
						ok = true
						break
					}
				}
			}
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "success": false, "error": "invalid token"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"role": req.Role}})
	})

	// user-scoped instance APIs (access by instance token)
	r.GET("/instances/me", middleware.APIKey(""), func(c *gin.Context) {
		token := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "success": false, "error": "missing X-API-Key"})
			return
		}
		out := make([]store.Instance, 0)
		for _, inst := range instances.List() {
			if inst.Token == token {
				out = append(out, inst)
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": out})
	})

	getMine := func(c *gin.Context) (store.Instance, bool) {
		token := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "success": false, "error": "missing X-API-Key"})
			return store.Instance{}, false
		}
		id := c.Param("id")
		inst, ok := instances.Get(id)
		if !ok || inst.Token != token {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "success": false, "error": "forbidden"})
			return store.Instance{}, false
		}
		return inst, true
	}

	r.GET("/instances/me/:id", middleware.APIKey(""), func(c *gin.Context) {
		inst, ok := getMine(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": inst})
	})

	r.POST("/instances/me/:id/session/connect", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok {
			return
		}
		if err := svc.Connect(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()})
			return
		}
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = true })
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"instance": inst, "qr": qrPayload(svc.QR())}})
	})
	r.GET("/instances/me/:id/session/qr", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"qr": qrPayload(svc.QR())}})
	})
	r.POST("/instances/me/:id/session/disconnect", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok {
			return
		}
		_ = svc.Disconnect()
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = false })
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": inst})
	})
	r.POST("/instances/me/:id/session/logout", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok {
			return
		}
		_ = svc.Logout()
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = false; i.LoggedIn = false; i.JID = "" })
		c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": inst})
	})
	// user config on owned instance
	r.POST("/instances/me/:id/config/webhook", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req updateWebhookReq
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.Webhook = req.Webhook; i.Events = req.Events })
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
	})
	r.POST("/instances/me/:id/config/history", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req struct{ History int `json:"history"` }
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.History = req.History })
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
	})
	r.POST("/instances/me/:id/config/proxy", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req struct{ Enabled bool `json:"enabled"`; ProxyURL string `json:"proxy_url"` }
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.Proxy.Enabled = req.Enabled; i.Proxy.ProxyURL = req.ProxyURL })
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
	})
	r.POST("/instances/me/:id/config/s3", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req store.S3Config
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.S3 = req })
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
	})
	r.POST("/instances/me/:id/config/hmac", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req struct{ HMACKey string `json:"hmac_key" binding:"required"` }
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		inst, _ := instances.Update(c.Param("id"), func(i *store.Instance) { i.HMACKey = req.HMACKey })
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
	})


	r.GET("/instances/me/:id/contacts", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":[]any{}})
	})
	r.GET("/instances/me/:id/groups", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":[]any{}})
	})
	r.POST("/instances/me/:id/groups/join", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req struct{ Invite string `json:"invite" binding:"required"` }
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"details":"join-group queued","invite":req.Invite}})
	})
	r.POST("/instances/me/:id/groups/create", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req struct{ Subject string `json:"subject" binding:"required"`; Participants []string `json:"participants"` }
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"details":"create-group queued","subject":req.Subject,"participants":req.Participants}})
	})
	r.POST("/instances/me/:id/chat/send/text", middleware.APIKey(""), func(c *gin.Context) {
		if _, ok := getMine(c); !ok { return }
		var req sendTextReq
		if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		if outbox != nil && outbox.Enabled() {
			qid, err := outbox.Enqueue(c.Param("id"), store.OutboxPayload{Phone:req.Phone, Message:req.Message}, 5)
			if err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"status":"queued","queue_id":qid}})
			return
		}
		id, err := svc.SendText(req.Phone, req.Message)
		if err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"message_id":id}})
	})

	r.GET("/queue/:id", func(c *gin.Context) {
		if outbox == nil || !outbox.Enabled() { c.JSON(http.StatusNotImplemented, gin.H{"code":501,"success":false,"error":"queue disabled"}); return }
		id := c.Param("id")
		it, ok := outbox.Get(id)
		if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"queue item not found"}); return }
		if c.GetHeader("Authorization") == "Bearer "+adminToken {
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":it}); return
		}
		// user token ownership check
		tok := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if tok == "" { c.JSON(http.StatusUnauthorized, gin.H{"code":401,"success":false,"error":"missing auth"}); return }
		inst, ok := instances.Get(it.InstanceID)
		if !ok || inst.Token != tok { c.JSON(http.StatusForbidden, gin.H{"code":403,"success":false,"error":"forbidden"}); return }
		c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":it})
	})

	user := r.Group("/", middleware.APIKey(userAPIKey))
	{
		user.GET("/session/status", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": svc.Status()}) })
		user.POST("/session/connect", func(c *gin.Context) {
			if err := svc.Connect(); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "connected", "qr": qrPayload(svc.QR())}})
		})
		user.POST("/session/disconnect", func(c *gin.Context) { _ = svc.Disconnect(); c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "disconnected"}}) })
		user.POST("/session/logout", func(c *gin.Context) { _ = svc.Logout(); c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "logged out"}}) })
		user.GET("/session/qr", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"qr": qrPayload(svc.QR())}}) })

		user.GET("/webhook", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"webhook": "", "events": []string{"All"}, "active": false}}) })
		user.POST("/webhook", func(c *gin.Context) { var body map[string]any; _ = c.BindJSON(&body); c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": body}) })
		user.DELETE("/webhook", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "webhook deleted"}}) })

		user.POST("/chat/send/text", func(c *gin.Context) {
			var req sendTextReq
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()}); return }
			id, err := svc.SendText(req.Phone, req.Message)
			if err != nil { c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"message_id": id}})
		})
		user.POST("/chat/send/image", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "image endpoint scaffolded"}}) })
		user.POST("/chat/send/document", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"details": "document endpoint scaffolded"}}) })
	}

	admin := r.Group("/admin", middleware.AdminToken(adminToken))
	{
		admin.GET("/users", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": users.List()}) })
		admin.POST("/users", func(c *gin.Context) {
			var req createUserReq
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()}); return }
			u := users.Create(req.Name, req.Token)
			c.JSON(http.StatusCreated, gin.H{"code": 201, "success": true, "data": u})
		})
		admin.DELETE("/users/:id", func(c *gin.Context) {
			if ok := users.Delete(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code": 404, "success": false, "error": "user not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"id": c.Param("id")}})
		})

		admin.GET("/instances", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": instances.List()}) })
		admin.POST("/instances", func(c *gin.Context) {
			var req createInstanceReq
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code": 400, "success": false, "error": err.Error()}); return }
			inst := instances.Create(store.Instance{Name: req.Name, Token: req.Token, Webhook: req.Webhook, Events: req.Events, History: req.History, HMACKey: req.HMACKey, Proxy: store.ProxyConfig{Enabled: req.ProxyOn, ProxyURL: req.ProxyURL}, S3: store.S3Config{Enabled: req.S3Enabled}})
			c.JSON(http.StatusCreated, gin.H{"code": 201, "success": true, "data": inst})
		})
		admin.GET("/instances/:id", func(c *gin.Context) {
			inst, ok := instances.Get(c.Param("id")); if !ok { c.JSON(http.StatusNotFound, gin.H{"code": 404, "success": false, "error": "instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": inst})
		})
		admin.DELETE("/instances/:id", func(c *gin.Context) {
			if ok := instances.Delete(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code": 404, "success": false, "error": "instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code": 200, "success": true, "data": gin.H{"id": c.Param("id")}})
		})
		admin.POST("/instances/:id/session/connect", func(c *gin.Context) {
			if err := svc.Connect(); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = true })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"instance":inst,"qr":qrPayload(svc.QR())}})
		})
		admin.GET("/instances/:id/session/qr", func(c *gin.Context) {
			inst, ok := instances.Get(c.Param("id"))
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"instance":inst,"qr":qrPayload(svc.QR())}})
		})
		admin.POST("/instances/:id/session/disconnect", func(c *gin.Context) {
			_ = svc.Disconnect()
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = false })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
		admin.POST("/instances/:id/session/logout", func(c *gin.Context) {
			_ = svc.Logout()
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.Connected = false; i.LoggedIn = false; i.JID = "" })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})


		admin.GET("/instances/:id/contacts", func(c *gin.Context) {
			if _, ok := instances.Get(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":[]any{}})
		})
		admin.GET("/instances/:id/groups", func(c *gin.Context) {
			if _, ok := instances.Get(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":[]any{}})
		})
		admin.POST("/instances/:id/groups/join", func(c *gin.Context) {
			if _, ok := instances.Get(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			var req struct{ Invite string `json:"invite" binding:"required"` }
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"details":"join-group queued","invite":req.Invite}})
		})
		admin.POST("/instances/:id/groups/create", func(c *gin.Context) {
			if _, ok := instances.Get(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			var req struct{ Subject string `json:"subject" binding:"required"`; Participants []string `json:"participants"` }
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"details":"create-group queued","subject":req.Subject,"participants":req.Participants}})
		})
		admin.POST("/instances/:id/chat/send/text", func(c *gin.Context) {
			if _, ok := instances.Get(c.Param("id")); !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			var req sendTextReq
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			if outbox != nil && outbox.Enabled() {
				qid, err := outbox.Enqueue(c.Param("id"), store.OutboxPayload{Phone:req.Phone, Message:req.Message}, 5)
				if err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
				c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"status":"queued","queue_id":qid}})
				return
			}
			id, err := svc.SendText(req.Phone, req.Message)
			if err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":gin.H{"message_id":id}})
		})

		admin.POST("/instances/:id/config/webhook", func(c *gin.Context) {
			var req updateWebhookReq
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.Webhook = req.Webhook; i.Events = req.Events })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
		admin.POST("/instances/:id/config/history", func(c *gin.Context) {
			var req struct{ History int `json:"history"` }
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.History = req.History })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
		admin.POST("/instances/:id/config/proxy", func(c *gin.Context) {
			var req struct{ Enabled bool `json:"enabled"`; ProxyURL string `json:"proxy_url"` }
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.Proxy.Enabled = req.Enabled; i.Proxy.ProxyURL = req.ProxyURL })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
		admin.POST("/instances/:id/config/s3", func(c *gin.Context) {
			var req store.S3Config
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.S3 = req })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
		admin.POST("/instances/:id/config/hmac", func(c *gin.Context) {
			var req struct{ HMACKey string `json:"hmac_key" binding:"required"` }
			if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"code":400,"success":false,"error":err.Error()}); return }
			inst, ok := instances.Update(c.Param("id"), func(i *store.Instance) { i.HMACKey = req.HMACKey })
			if !ok { c.JSON(http.StatusNotFound, gin.H{"code":404,"success":false,"error":"instance not found"}); return }
			c.JSON(http.StatusOK, gin.H{"code":200,"success":true,"data":inst})
		})
	}

	return r
}
