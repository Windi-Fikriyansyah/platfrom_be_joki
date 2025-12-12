package middleware

import (
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func JWTFromCookie(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenStr := c.Cookies("jm_token")
		if tokenStr == "" {
			return fiber.ErrUnauthorized
		}

		token, err := jwt.ParseWithClaims(tokenStr, &utils.Claims{}, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			return fiber.ErrUnauthorized
		}

		c.Locals("user", token)
		return c.Next()
	}
}
