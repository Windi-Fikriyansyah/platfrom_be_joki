package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func getUserUUID(c *fiber.Ctx) (uuid.UUID, error) {
	v := c.Locals("userId")
	if v == nil {
		return uuid.Nil, fmt.Errorf("unauthorized")
	}

	switch t := v.(type) {
	case uuid.UUID:
		return t, nil
	case string:
		return uuid.Parse(t)
	case []byte:
		return uuid.ParseBytes(t)
	default:
		return uuid.Nil, fmt.Errorf("invalid userId type: %T", v)
	}
}
