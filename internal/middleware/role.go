package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

func RequireRoles(allowed ...string) fiber.Handler {
	allowedSet := map[string]bool{}
	for _, r := range allowed {
		allowedSet[strings.ToLower(r)] = true
	}

	return func(c *fiber.Ctx) error {
		raw := c.Locals("user")
		if raw == nil {
			return fiber.ErrUnauthorized
		}

		token, ok := raw.(*jwt.Token)
		if !ok || token == nil {
			return fiber.ErrUnauthorized
		}

		claims, ok := token.Claims.(*utils.Claims)
		if !ok {
			return fiber.ErrUnauthorized
		}

		role := strings.ToLower(strings.TrimSpace(claims.Role))
		if !allowedSet[role] {
			return fiber.NewError(fiber.StatusForbidden, "forbidden: insufficient role")
		}

		return c.Next()
	}
}
