package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

type ProductHandler struct {
	DB *gorm.DB
}

func NewProductHandler(db *gorm.DB) *ProductHandler {
	return &ProductHandler{DB: db}
}

// ==== REQUEST STRUCTS ====

type PackageReq struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	DeliveryDays int      `json:"delivery_days"` // dalam hari
	Revisions    int      `json:"revisions"`
	Price        int64    `json:"price"`
	Benefits     []string `json:"benefits"`
}

type PortfolioImageReq struct {
	FileName    string `json:"file_name"`
	Description string `json:"description"`
}

type ProductBasicReq struct {
	Title     string `json:"title"`
	Category  string `json:"category"`
	BasePrice int64  `json:"base_price"`

	VisibilityDescription string         `json:"visibility_description"`
	CoverURL              string         `json:"cover_url"`       // kalau nanti sudah ada upload cover
	CoverTransform        map[string]any `json:"cover_transform"` // { scale, pos: {x,y}, flipH, flipV }

	Basic    PackageReq `json:"basic"`
	Standard PackageReq `json:"standard"`
	Premium  PackageReq `json:"premium"`

	PortfolioVideoURL string              `json:"portfolio_video_url"`
	PortfolioImages   []PortfolioImageReq `json:"portfolio_images"`

	Status string `json:"status"` // "draft" / "review" dll
}

// ==== HANDLER ====

func (h *ProductHandler) CreateBasic(c *fiber.Ctx) error {
	var req ProductBasicReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Body request tidak valid",
		})
	}

	// Validasi sederhana
	if req.Title == "" || req.Category == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Judul dan kategori wajib diisi",
		})
	}

	// Ambil user dari JWT (sama seperti /me)
	uid := c.Locals("userId")

	var user models.User
	if err := h.DB.First(&user, "id = ?", uid).Error; err != nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "User tidak ditemukan",
		})
	}

	// Siapkan JSON untuk packages
	packagesPayload := map[string]PackageReq{
		"basic":    req.Basic,
		"standard": req.Standard,
		"premium":  req.Premium,
	}
	packagesJSON, err := json.Marshal(packagesPayload)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal memproses data paket",
		})
	}

	// Siapkan JSON untuk portfolio
	portfolioPayload := map[string]any{
		"video_url": req.PortfolioVideoURL,
		"images":    req.PortfolioImages,
	}
	portfolioJSON, err := json.Marshal(portfolioPayload)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal memproses data portofolio",
		})
	}

	// JSON cover transform
	var coverTransformJSON []byte
	if req.CoverTransform != nil {
		coverTransformJSON, err = json.Marshal(req.CoverTransform)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Gagal memproses data cover",
			})
		}
	}

	status := req.Status
	if status == "" {
		status = "draft"
	}

	product := models.Product{
		UserID:                user.ID,
		Title:                 req.Title,
		Category:              req.Category,
		BasePrice:             req.BasePrice,
		VisibilityDescription: req.VisibilityDescription,
		CoverURL:              req.CoverURL,
		CoverTransform:        datatypes.JSON(coverTransformJSON),
		Packages:              datatypes.JSON(packagesJSON),
		Portfolio:             datatypes.JSON(portfolioJSON),
		Status:                status,
	}

	if err := h.DB.Create(&product).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal menyimpan produk",
		})
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"success": true,
		"message": "Produk berhasil disimpan",
		"data": fiber.Map{
			"id":       product.ID,
			"status":   product.Status,
			"title":    product.Title,
			"category": product.Category,
		},
	})
}

func (h *ProductHandler) UploadCover(c *fiber.Ctx) error {
	uid := c.Locals("userId")

	// pastikan user valid (opsional tapi bagus)
	var user models.User
	if err := h.DB.First(&user, "id = ?", uid).Error; err != nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "User tidak ditemukan",
		})
	}

	file, err := c.FormFile("cover")
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "File cover tidak ditemukan",
		})
	}

	// Validasi sederhana
	if file.Size <= 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Ukuran file tidak valid",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Format gambar tidak didukung",
		})
	}

	// Pastikan folder ada
	uploadDir := "./uploads/covers"
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal membuat folder upload",
		})
	}

	// Nama file unik
	filename := fmt.Sprintf("cover_%v_%d%s", uid, time.Now().UnixNano(), ext)
	savePath := filepath.Join(uploadDir, filename)

	if err := c.SaveFile(file, savePath); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal menyimpan file cover",
		})
	}

	// URL publik (app.Static("/uploads", "./uploads"))
	// Kalau APP_BASE_URL di-set, pakai itu
	base := os.Getenv("APP_BASE_URL") // misal: http://localhost:8080
	publicPath := "/uploads/covers/" + filename

	fullURL := publicPath
	if base != "" {
		fullURL = strings.TrimRight(base, "/") + publicPath
	}

	return c.JSON(fiber.Map{
		"success": true,
		"url":     fullURL,
	})
}

func (h *ProductHandler) ListMine(c *fiber.Ctx) error {
	uid := c.Locals("userId")

	var products []models.Product
	if err := h.DB.
		Where("user_id = ?", uid).
		Order("created_at DESC").
		Find(&products).Error; err != nil {

		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal mengambil produk",
		})
	}

	// Optional: kalau mau lebih ringan, kirim field penting saja
	out := make([]fiber.Map, 0, len(products))
	for _, p := range products {
		enc, _ := utils.EncryptID(p.ID, os.Getenv("ID_ENCRYPT_KEY"))
		out = append(out, fiber.Map{
			"id":         enc,
			"real_id":    p.ID,
			"title":      p.Title,
			"category":   p.Category,
			"base_price": p.BasePrice,
			"cover_url":  p.CoverURL,
			"status":     p.Status,
			"created_at": p.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    out,
	})
}

func (h *ProductHandler) GetOne(c *fiber.Ctx) error {
	encID := c.Params("id")
	rawID, err := utils.DecryptID(encID, os.Getenv("ID_ENCRYPT_KEY"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid product ID",
		})
	}

	var product models.Product

	if err := h.DB.First(&product, "id = ?", rawID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Produk tidak ditemukan",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    product,
	})
}

func (h *ProductHandler) UploadPortfolioImage(c *fiber.Ctx) error {
	uid := c.Locals("userId")

	file, err := c.FormFile("image")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "File tidak ditemukan",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Format tidak didukung",
		})
	}

	uploadDir := "./uploads/portfolio"
	os.MkdirAll(uploadDir, 0755)

	filename := fmt.Sprintf("p_%v_%d%s", uid, time.Now().UnixNano(), ext)
	savePath := filepath.Join(uploadDir, filename)

	if err := c.SaveFile(file, savePath); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal menyimpan gambar",
		})
	}

	base := os.Getenv("APP_BASE_URL")
	publicURL := "/uploads/portfolio/" + filename
	if base != "" {
		publicURL = base + publicURL
	}

	return c.JSON(fiber.Map{
		"success": true,
		"url":     publicURL,
	})
}

func (h *ProductHandler) UpdateProduct(c *fiber.Ctx) error {
	encID := c.Params("id")

	rawID, err := utils.DecryptID(encID, os.Getenv("ID_ENCRYPT_KEY"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid product ID",
		})
	}

	uid := c.Locals("userId")

	var product models.Product
	if err := h.DB.First(&product, "id = ? AND user_id = ?", rawID, uid).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Produk tidak ditemukan",
		})
	}

	var req ProductBasicReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Body request tidak valid",
		})
	}

	// packages
	packagesPayload := map[string]PackageReq{
		"basic":    req.Basic,
		"standard": req.Standard,
		"premium":  req.Premium,
	}
	packagesJSON, _ := json.Marshal(packagesPayload)

	// portfolio
	portfolioPayload := map[string]any{
		"video_url": req.PortfolioVideoURL,
		"images":    req.PortfolioImages,
	}
	portfolioJSON, _ := json.Marshal(portfolioPayload)

	var coverTransformJSON []byte
	if req.CoverTransform != nil {
		coverTransformJSON, _ = json.Marshal(req.CoverTransform)
	}

	product.Title = req.Title
	product.Category = req.Category
	product.BasePrice = req.BasePrice
	product.VisibilityDescription = req.VisibilityDescription
	product.CoverURL = req.CoverURL
	product.CoverTransform = datatypes.JSON(coverTransformJSON)
	product.Packages = datatypes.JSON(packagesJSON)
	product.Portfolio = datatypes.JSON(portfolioJSON)

	if req.Status != "" {
		product.Status = req.Status
	}

	if err := h.DB.Save(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal memperbarui produk",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Produk berhasil diperbarui",
	})
}

func (h *ProductHandler) Delete(c *fiber.Ctx) error {
	uid := c.Locals("userId")

	encID := c.Params("id")
	rawID, err := utils.DecryptID(encID, os.Getenv("ID_ENCRYPT_KEY"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "ID produk tidak valid",
		})
	}

	var product models.Product
	if err := h.DB.
		Where("id = ? AND user_id = ?", rawID, uid).
		First(&product).Error; err != nil {

		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Produk tidak ditemukan",
		})
	}

	if err := h.DB.Delete(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal menghapus produk",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Produk berhasil dihapus",
	})
}

func (h *ProductHandler) ListPublic(c *fiber.Ctx) error {
	type Result struct {
		ID        uint
		Title     string
		Category  string
		BasePrice int64
		CoverURL  string
		UserID    uuid.UUID

		SystemName     string
		FreelancerType models.FreelancerType
		PhotoURL       string
		AvgRating      float64
		ReviewCount    int64
		Sold           int64
	}

	// ===== FILTER =====
	qSearch := c.Query("q")
	category := c.Query("cat")
	minPrice := c.QueryInt("min", 0)
	maxPrice := c.QueryInt("max", 0)
	sortParam := c.Query("sort") // latest | price_low | price_high

	q := h.DB.
		Table("products").
		Select(`
			products.id,
			products.title,
			products.category,
			products.base_price,
			products.cover_url,
			products.user_id,
			fp.system_name,
			fp.freelancer_type,
			fp.photo_url,
			(SELECT COALESCE(AVG(rating), 0) FROM reviews r WHERE r.product_id = products.id) as avg_rating,
			(SELECT COUNT(*) FROM reviews r WHERE r.product_id = products.id) as review_count,
			(SELECT COUNT(*) FROM job_offers jo WHERE jo.product_id = products.id AND jo.status = 'completed') as sold
		`).
		Joins(`
			LEFT JOIN freelancer_profiles fp 
			ON fp.user_id = products.user_id
		`).
		Where("products.status = ?", "published")

	if qSearch != "" {
		q = q.Where("LOWER(products.title) LIKE ?", "%"+strings.ToLower(qSearch)+"%")
	}
	if category != "" {
		q = q.Where("products.category = ?", category)
	}
	if minPrice > 0 {
		q = q.Where("products.base_price >= ?", minPrice)
	}
	if maxPrice > 0 {
		q = q.Where("products.base_price <= ?", maxPrice)
	}

	// ===== SORTING =====
	switch sortParam {
	case "price_low":
		q = q.Order("products.base_price ASC")
	case "price_high":
		q = q.Order("products.base_price DESC")
	default:
		// Default latest
		q = q.Order("products.created_at DESC")
	}

	// ===== PAGINATION =====
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var totalItems int64
	// Count total items using a separate session to avoid polluting the main query logic
	// We need to count based on the filters, but 'Count' in GORM might modify the query.
	// So we create a countQuery that excludes Limit/Offset/Order.
	// However, GORM's Count method should handle this, but to be 100% safe with complex Selects:
	if err := h.DB.Table("products").
		Joins("LEFT JOIN freelancer_profiles fp ON fp.user_id = products.user_id").
		Where("products.status = ?", "published").
		Where(func(db *gorm.DB) *gorm.DB {
			if qSearch != "" {
				db = db.Where("LOWER(products.title) LIKE ?", "%"+strings.ToLower(qSearch)+"%")
			}
			if category != "" {
				db = db.Where("products.category = ?", category)
			}
			if minPrice > 0 {
				db = db.Where("products.base_price >= ?", minPrice)
			}
			if maxPrice > 0 {
				db = db.Where("products.base_price <= ?", maxPrice)
			}
			return db
		}(h.DB)).
		Count(&totalItems).Error; err != nil {

		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal menghitung layanan",
		})
	}

	var rows []Result
	if err := q.
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {

		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal mengambil layanan",
		})
	}

	out := make([]fiber.Map, 0, len(rows))
	for _, r := range rows {

		encID, err := utils.EncryptID(r.ID, os.Getenv("ID_ENCRYPT_KEY"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Gagal mengenkripsi ID produk",
			})
		}

		name := r.SystemName
		if name == "" {
			name = "Mentor"
		}

		level := "Freelancer"
		switch r.FreelancerType {
		case models.FreelancerFullTime:
			level = "Full Time"
		case models.FreelancerPartTime:
			level = "Part Time"
		}

		out = append(out, fiber.Map{
			"id":           encID,
			"title":        r.Title,
			"category":     r.Category,
			"price":        r.BasePrice,
			"cover":        r.CoverURL,
			"rating":       r.AvgRating,
			"sold":         r.Sold,
			"review_count": r.ReviewCount,
			"seller": fiber.Map{
				"name":      name,
				"title":     "Freelancer",
				"level":     level,
				"photo_url": r.PhotoURL,
			},
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    out,
		"meta": fiber.Map{
			"page":        page,
			"limit":       limit,
			"total_items": totalItems,
			"total_pages": int(math.Ceil(float64(totalItems) / float64(limit))),
		},
	})
}

func (h *ProductHandler) GetDetail(c *fiber.Ctx) error {
	encID := c.Params("id")

	// Decrypt ID
	rawID, err := utils.DecryptID(encID, os.Getenv("ID_ENCRYPT_KEY"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid product ID",
			"debug":   err.Error(),
		})
	}

	var product models.Product

	// Coba dulu tanpa filter status untuk debug
	if err := h.DB.First(&product, "id = ?", rawID).Error; err != nil {

		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Produk tidak ditemukan",
			"debug": fiber.Map{
				"id":    rawID,
				"error": err.Error(),
			},
		})
	}

	// Cek status
	if product.Status != "published" {

		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Produk belum dipublikasikan",
			"debug": fiber.Map{
				"status": product.Status,
			},
		})
	}

	// Get freelancer profile
	var profile models.FreelancerProfile
	if err := h.DB.Where("user_id = ?", product.UserID).First(&profile).Error; err != nil {

	}

	// Parse packages JSON
	var packages map[string]interface{}
	if len(product.Packages) > 0 {
		if err := json.Unmarshal(product.Packages, &packages); err != nil {

		}
	}

	// Parse portfolio JSON
	var portfolio map[string]interface{}
	if len(product.Portfolio) > 0 {
		if err := json.Unmarshal(product.Portfolio, &portfolio); err != nil {

		}
	}

	// Parse cover transform JSON
	var coverTransform map[string]interface{}
	if len(product.CoverTransform) > 0 {
		if err := json.Unmarshal(product.CoverTransform, &coverTransform); err != nil {

		}
	}

	// Build freelancer data
	freelancerName := profile.SystemName
	if freelancerName == "" {
		freelancerName = "Freelancer"
	}

	freelancerLevel := "Freelancer"
	switch profile.FreelancerType {
	case models.FreelancerFullTime:
		freelancerLevel = "Full Time"
	case models.FreelancerPartTime:
		freelancerLevel = "Part Time"
	}

	// Calculate rating stats
	var ratingStats struct {
		AvgRating   float64
		ReviewCount int64
	}
	h.DB.Model(&models.Review{}).
		Where("product_id = ?", product.ID).
		Select("COALESCE(AVG(rating), 0) as avg_rating, COUNT(*) as review_count").
		Scan(&ratingStats)

	var soldCount int64
	h.DB.Model(&models.JobOffer{}).
		Where("product_id = ? AND status = ?", product.ID, "completed").
		Count(&soldCount)

	log.Printf("[GetOnePublic] Success! Returning product data")

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"id":                     product.ID,
			"title":                  product.Title,
			"category":               product.Category,
			"base_price":             product.BasePrice,
			"visibility_description": product.VisibilityDescription,
			"cover_url":              product.CoverURL,
			"cover_transform":        coverTransform,
			"packages":               packages,
			"portfolio":              portfolio,
			"status":                 product.Status,
			"rating":                 ratingStats.AvgRating,
			"review_count":           ratingStats.ReviewCount,
			"sold":                   soldCount,
			"freelancer": fiber.Map{
				"id":        product.UserID,
				"name":      freelancerName,
				"title":     "Freelancer",
				"photo_url": profile.PhotoURL,
				"level":     freelancerLevel,
			},
			"created_at": product.CreatedAt,
			"updated_at": product.UpdatedAt,
		},
	})
}

func (h *ProductHandler) GetReviews(c *fiber.Ctx) error {
	encID := c.Params("id")
	rawID, err := utils.DecryptID(encID, os.Getenv("ID_ENCRYPT_KEY"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Invalid product ID",
		})
	}

	var reviews []models.Review
	if err := h.DB.
		Where("product_id = ?", rawID).
		Preload("Client"). // Load reviewer info
		Order("created_at DESC").
		Find(&reviews).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal mengambil ulasan",
		})
	}

	out := make([]fiber.Map, 0, len(reviews))
	for _, r := range reviews {
		reviewerName := "Pengguna"
		reviewerPhoto := ""
		if r.Client != nil {
			reviewerName = r.Client.Name
		}

		out = append(out, fiber.Map{
			"id":         r.ID,
			"rating":     r.Rating,
			"comment":    r.Comment,
			"created_at": r.CreatedAt,
			"reviewer": fiber.Map{
				"name":      reviewerName,
				"photo_url": reviewerPhoto,
			},
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    out,
	})
}
