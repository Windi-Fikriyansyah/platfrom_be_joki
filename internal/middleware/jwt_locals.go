package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

func AttachJWTLocals() fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw := c.Locals("user")
		if raw == nil {
			return fiber.ErrUnauthorized
		}

		token, ok := raw.(*jwt.Token)
		if !ok || token == nil {
			return fiber.ErrUnauthorized
		}

		// ⬅⬅ CLAIMS SEKARANG HARUS utils.Claims
		claims, ok := token.Claims.(*utils.Claims)
		if !ok {
			return fiber.ErrUnauthorized
		}

		uid := strings.TrimSpace(claims.UserID)
		role := strings.ToLower(strings.TrimSpace(claims.Role))

		if uid == "" {
			return fiber.ErrUnauthorized
		}

		c.Locals("userId", uid)
		c.Locals("role", role)

		return c.Next()
	}
}
