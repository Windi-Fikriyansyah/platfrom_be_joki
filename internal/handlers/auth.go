package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret string
	Expires   int
}


type RegisterReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Phone    string `json:"phone"`
	Role     string `json:"role"` // client / freelancer (admin jangan dari publik)
}


type FieldErrors map[string][]string

func (e FieldErrors) Add(field, msg string) {
	e[field] = append(e[field], msg)
}

func validationFail(c *fiber.Ctx, errs FieldErrors) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": false,
		"message": "Validation error",
		"errors":  errs,
	})
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "invalid body",
		})
	}

	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	phone := strings.TrimSpace(req.Phone) 
	password := strings.TrimSpace(req.Password)

	role := string(models.RoleClient)

	// --- Validasi basic
	errors := FieldErrors{}


	if name == "" {
		errors.Add("name", "Nama wajib diisi")
	}
	if email == "" {
		errors.Add("email", "Email wajib diisi")
	} else if !strings.Contains(email, "@") {
		errors.Add("email", "Format email tidak valid")
	}
	if password == "" {
		errors.Add("password", "Password wajib diisi")
	} else if len(password) < 6 {
		errors.Add("password", "Password minimal 6 karakter")
	}

	if phone != "" && len(phone) < 8 {
		errors.Add("phone", "No. HP tidak valid")
	}

	if len(errors) > 0 {
		return validationFail(c, errors)
	}



	// --- Cek email sudah ada
	var existing models.User
	if err := h.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		errs := FieldErrors{}
		errs.Add("email", "Email sudah terdaftar")
		return validationFail(c, errs)
	} else if err != nil && err != gorm.ErrRecordNotFound {
		// error DB selain "record not found"
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Terjadi kesalahan server",
		})
	}

	if phone != "" {
		var byPhone models.User
		if err := h.DB.Where("phone = ?", phone).First(&byPhone).Error; err == nil {
			errs := FieldErrors{}
			errs.Add("phone", "No. HP sudah terdaftar")
			return validationFail(c, errs)
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Terjadi kesalahan server",
			})
		}
	}

	// --- Hash password
	pw, err := utils.HashPassword(password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal memproses password",
		})
	}

	u := models.User{
		Name:     name,
		Email:    email,
		Password: pw,
		Role:     models.Role(role), // <-- PASTI CLIENT
		IsActive: true,
		Phone:    phone,
	}

	if err := h.DB.Create(&u).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Gagal register",
		})
	}

	// --- Buat token
	token, err := utils.SignJWT(h.JWTSecret, u.ID.String(), string(u.Role), h.Expires)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "Gagal membuat token",
		})
	}

	c.Cookie(&fiber.Cookie{
    Name:     "jm_token",
    Value:    token,
    Path:     "/",
    HTTPOnly: true,
    Secure:   false,
    SameSite: "Lax",
    MaxAge:   h.Expires * 60,
})

return c.Status(fiber.StatusCreated).JSON(fiber.Map{
    "success": true,
    "message": "Register berhasil",
    "data": fiber.Map{
        "user": fiber.Map{
            "id":    u.ID,
            "name":  u.Name,
            "email": u.Email,
            "phone": u.Phone,
            "role":  u.Role,
        },
    },
})
}


type LoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
    var req LoginReq
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusOK).JSON(fiber.Map{
            "success": false,
            "message": "Invalid body",
        })
    }

    email := strings.ToLower(strings.TrimSpace(req.Email))
    password := strings.TrimSpace(req.Password)

    errors := FieldErrors{}
    if email == "" {
        errors.Add("email", "Email wajib diisi")
    }
    if password == "" {
        errors.Add("password", "Password wajib diisi")
    }

    if len(errors) > 0 {
        return validationFail(c, errors)
    }

    var u models.User
    err := h.DB.Where("email = ?", email).First(&u).Error

    if err != nil {
        // Email tidak ditemukan -> tetap 200 agar FE tidak error
        return c.Status(fiber.StatusOK).JSON(fiber.Map{
            "success": false,
            "message": "Email atau password salah",
        })
    }

    if !u.IsActive {
        return c.Status(fiber.StatusOK).JSON(fiber.Map{
            "success": false,
            "message": "Akun tidak aktif",
        })
    }

    if !utils.CheckPassword(u.Password, password) {
        return c.Status(fiber.StatusOK).JSON(fiber.Map{
            "success": false,
            "message": "Email atau password salah",
        })
    }

    token, err := utils.SignJWT(h.JWTSecret, u.ID.String(), string(u.Role), h.Expires)
    if err != nil {
        return c.Status(fiber.StatusOK).JSON(fiber.Map{
            "success": false,
            "message": "Gagal membuat token",
        })
    }

    c.Cookie(&fiber.Cookie{
        Name:     "jm_token",
        Value:    token,
        Path:     "/",
        HTTPOnly: true,
        Secure:   false,
        SameSite: "Lax",
        MaxAge:   h.Expires * 60,
    })

    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "success": true,
        "message": "Login berhasil",
        "data": fiber.Map{
            "user": fiber.Map{
                "id":    u.ID,
                "name":  u.Name,
                "email": u.Email,
                "role":  u.Role,
            },
        },
    })
}


func (h *AuthHandler) Logout(c *fiber.Ctx) error {
    c.Cookie(&fiber.Cookie{
        Name:     "jm_token",
        Value:    "",
        Path:     "/",
        MaxAge:   -1,     // hapus cookie
        HTTPOnly: true,
        Secure:   false,   // gunakan false jika development HTTP
        SameSite: "Lax",
    })

    return c.JSON(fiber.Map{
        "success": true,
        "message": "Logout berhasil",
    })
}
