package main

import (
	"context"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/websocket/v2"
	"github.com/joho/godotenv"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/config"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/db"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/handlers"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/middleware"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/realtime"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	gdb, err := db.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}

	rdb := realtime.NewRedis()

	hub := realtime.NewHub()
	go hub.Run()

	chatH := handlers.NewChatHandler(gdb, hub, rdb)

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("Redis TIDAK dipakai / TIDAK connect:", err)
	}
	log.Println("Redis AKTIF & DIPAKAI oleh backend ✅")

	if err := gdb.AutoMigrate(&models.User{}, &models.FreelancerProfile{}, &models.Product{}, &models.Conversation{},
		&models.Message{},
		&models.ConversationMemberRead{},
		&models.JobOffer{}); err != nil {
		log.Fatal(err)
	}

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://127.0.0.1:3000, http://localhost:3000",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		ExposeHeaders:    "Content-Length",
		AllowCredentials: true, // ubah ke true kalau pakai cookie
	}))

	// (opsional) biar preflight selalu kejawab
	app.Options("/*", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})

	app.Static("/uploads", "./uploads")
	authH := &handlers.AuthHandler{
		DB:        gdb,
		JWTSecret: cfg.JWTSecret,
		Expires:   cfg.JWTExpiresMin,
	}
	productH := handlers.NewProductHandler(gdb)
	categoryH := handlers.NewCategoryHandler(gdb)

	googleH := &handlers.GoogleOAuthHandler{
		DB:              gdb,
		JWTSecret:       cfg.JWTSecret,
		Expires:         cfg.JWTExpiresMin,
		GoogleClientID:  os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:    os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirect:  os.Getenv("GOOGLE_REDIRECT_URL"),
		FrontendBaseURL: os.Getenv("FRONTEND_BASE_URL"),
	}

	api := app.Group("/api")

	// public
	api.Post("/auth/register", authH.Register)
	api.Post("/auth/login", authH.Login)
	api.Post("/auth/logout", authH.Logout)
	api.Get("/auth/google/start", googleH.GoogleStart)
	api.Get("/auth/google/callback", googleH.GoogleCallback)
	api.Get("/categories", categoryH.GetCategories)
	api.Get("/products", productH.ListPublic)
	api.Get("/products/:id", productH.GetDetail)

	// protected (JWT)
	protected := api.Group("/",
		middleware.JWTFromCookie(cfg.JWTSecret), // ⬅️ baca token dari cookie
		middleware.AttachJWTLocals(),
	)

	protected.Get("/freelancer/profile/me",
		middleware.RequireRoles("freelancer"),
		func(c *fiber.Ctx) error {
			uid := c.Locals("userId")

			var profile models.FreelancerProfile
			if err := gdb.
				Where("user_id = ?", uid).
				First(&profile).Error; err != nil {

				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"success": false,
					"message": "Profil tidak ditemukan",
				})
			}

			return c.JSON(fiber.Map{
				"success": true,
				"data": fiber.Map{
					"system_name": profile.SystemName,
					"photo_url":   profile.PhotoURL,
				},
			})
		},
	)

	// contoh: siapa saya
	protected.Get("/me", func(c *fiber.Ctx) error {
		uid := c.Locals("userId")

		// Ambil user dari database
		var user models.User
		if err := gdb.First(&user, "id = ?", uid).Error; err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "User tidak ditemukan",
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"data": fiber.Map{
				"id":    user.ID,
				"name":  user.Name,
				"email": user.Email,
				"role":  user.Role,
			},
		})
	})

	// client only
	protected.Get("/client/orders",
		middleware.RequireRoles("client"),
		func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"msg": "client orders"}) },
	)

	// freelancer only
	protected.Get("/freelancer/jobs",
		middleware.RequireRoles("freelancer"),
		func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"msg": "freelancer jobs"}) },
	)
	protected.Post("/freelancer/products/basic",
		middleware.RequireRoles("freelancer"), // yang boleh buat produk: freelancer
		productH.CreateBasic,
	)

	protected.Get("/freelancer/products",
		middleware.RequireRoles("freelancer"),
		productH.ListMine,
	)

	chat := protected.Group("/chat")

	// Job Offer Handler
	offerH := handlers.NewJobOfferHandler(gdb, hub, rdb)

	// HTTP Endpoints
	chat.Post("/conversations", chatH.CreateOrGetConversation)
	chat.Get("/conversations", chatH.GetConversations)
	chat.Get("/conversations/:id/messages", chatH.GetMessages)
	chat.Post("/conversations/:id/messages", chatH.SendMessage)
	chat.Patch("/conversations/:id/read", chatH.MarkAsRead)

	// Job Offer Endpoints
	chat.Post("/conversations/:id/offers", offerH.CreateOffer)
	chat.Get("/conversations/:id/offers", offerH.GetOffers)
	protected.Get("/job-offers/:id", offerH.GetOffer)
	protected.Put("/job-offers/:id", offerH.UpdateOffer)
	protected.Patch("/job-offers/:id/status", offerH.UpdateStatus)

	// WebSocket endpoint (tanpa JWT middleware, autentikasi via query param)
	app.Get("/ws/chat", websocket.New(chatH.WebSocketHandler))

	protected.Get("/freelancer/products/:id", productH.GetOne)
	protected.Put(
		"/freelancer/products/:id",
		middleware.RequireRoles("freelancer"),
		productH.UpdateProduct,
	)
	protected.Delete("/freelancer/products/:id",
		middleware.RequireRoles("freelancer"),
		productH.Delete,
	)

	protected.Post("/freelancer/products/cover",
		middleware.RequireRoles("freelancer"),
		productH.UploadCover,
	)

	protected.Post("/freelancer/products/portfolio/image",
		middleware.RequireRoles("freelancer"),
		productH.UploadPortfolioImage,
	)

	// admin only
	protected.Get("/admin/users",
		middleware.RequireRoles("admin"),
		func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"msg": "admin users"}) },
	)

	fOnboard := handlers.NewFreelancerOnboardingHandler(
		gdb,
		"./uploads",
		os.Getenv("APP_BASE_URL"), // opsional, boleh kosong
		cfg.JWTSecret,
		cfg.JWTExpiresMin,
	)

	onb := protected.Group("/freelancer/onboarding", middleware.RequireRoles("client"))

	onb.Get("/", fOnboard.Get)
	onb.Post("/photo", fOnboard.UploadPhoto)
	onb.Patch("/profile", fOnboard.UpdateProfile)
	onb.Patch("/about", fOnboard.UpdateAbout)
	onb.Patch("/identity", fOnboard.UpdateIdentity)
	onb.Patch("/contact", fOnboard.UpdateContact)
	onb.Post("/submit", fOnboard.Submit)

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
