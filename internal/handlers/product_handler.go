package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
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
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DeliveryDays int     `json:"delivery_days"` // dalam hari
	Revisions   int      `json:"revisions"`
	Price       int64    `json:"price"`
	Benefits    []string `json:"benefits"`
}

type PortfolioImageReq struct {
	FileName    string `json:"file_name"`
	Description string `json:"description"`
}

type ProductBasicReq struct {
	Title     string `json:"title"`
	Category  string `json:"category"`
	BasePrice int64  `json:"base_price"`

	VisibilityDescription string                 `json:"visibility_description"`
	CoverURL              string                 `json:"cover_url"`        // kalau nanti sudah ada upload cover
	CoverTransform        map[string]any         `json:"cover_transform"`  // { scale, pos: {x,y}, flipH, flipV }

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
			"id":      product.ID,
			"status":  product.Status,
			"title":   product.Title,
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
			 "id":          enc,
    		"real_id":     p.ID,
			"title":      p.Title,
			"category":   p.Category,
			"base_price": p.BasePrice,
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
