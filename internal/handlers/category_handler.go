package handlers

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type CategoryHandler struct {
	DB *gorm.DB
}

func NewCategoryHandler(db *gorm.DB) *CategoryHandler {
	return &CategoryHandler{DB: db}
}

func (h *CategoryHandler) GetCategories(c *fiber.Ctx) error {
	var categories []string

	err := h.DB.
		Table("products").
		Where("status = ?", "published").
		Distinct("category").
		Pluck("category", &categories).
		Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"message": "Gagal mengambil kategori",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    categories,
	})
}
