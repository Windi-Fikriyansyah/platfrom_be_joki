package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

type GoogleOAuthHandler struct {
	DB              *gorm.DB
	JWTSecret       string
	Expires         int
	GoogleClientID  string
	GoogleSecret    string
	GoogleRedirect  string
	FrontendBaseURL string
}

func (h *GoogleOAuthHandler) oauthCfg() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.GoogleClientID,
		ClientSecret: h.GoogleSecret,
		RedirectURL:  h.GoogleRedirect,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"openid", "email", "profile"},
	}
}

func randomState(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (h *GoogleOAuthHandler) GoogleStart(c *fiber.Ctx) error {
	next := c.Query("next", "/")
	st := randomState(32)

	// simpan state + next di cookie sementara
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_state",
		Value:    st,
		Path:     "/",
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		MaxAge:   10 * 60,
	})
	c.Cookie(&fiber.Cookie{
		Name:     "oauth_next",
		Value:    next,
		Path:     "/",
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		MaxAge:   10 * 60,
	})

	authURL := h.oauthCfg().AuthCodeURL(st,
		oauth2.AccessTypeOffline,
	)

	return c.Redirect(authURL, http.StatusTemporaryRedirect)
}

type googleUserInfo struct {
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (h *GoogleOAuthHandler) GoogleCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Missing code/state")
	}

	stCookie := c.Cookies("oauth_state")
	next := c.Cookies("oauth_next")
	if next == "" {
		next = "/"
	}

	if stCookie == "" || stCookie != state {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid state")
	}

	// exchange code -> token
	tok, err := h.oauthCfg().Exchange(c.Context(), code)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to exchange code")
	}

	// ambil userinfo
	client := h.oauthCfg().Client(c.Context(), tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to fetch userinfo")
	}
	defer resp.Body.Close()

	var gu googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&gu); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to decode userinfo")
	}

	email := strings.ToLower(strings.TrimSpace(gu.Email))
	name := strings.TrimSpace(gu.Name)
	if email == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Email not found from Google")
	}

	// upsert user by email
	var u models.User
	err = h.DB.Where("email = ?", email).First(&u).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return c.Status(fiber.StatusInternalServerError).SendString("DB error")
	}

	if err == gorm.ErrRecordNotFound {
		// User model kamu mewajibkan Password not null,
		// jadi bikin password random (tidak dipakai untuk login manual).
		rawPass := randomState(24)
		hashed, _ := utils.HashPassword(rawPass)

		u = models.User{
			Name:     name,
			Email:    email,
			Password: hashed,
			Role:     models.RoleClient,
			IsActive: true,
			// workaround for unique index on phone:
			// use a dummy value if it's empty, or handled by DB as null (if it was nullable)
			// but here it's likely failing because "" is already taken by another user.
			Phone: fmt.Sprintf("google_%s", state[:10]),
		}
		if err := h.DB.Create(&u).Error; err != nil {
			log.Println("Error creating user via Google:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Gagal membuat akun: " + err.Error(),
			})
		}
	} else {
		// update nama kalau kosong / beda (opsional)
		if name != "" && u.Name != name {
			u.Name = name
			_ = h.DB.Save(&u).Error
		}
	}

	if !u.IsActive {
		// redirect ke FE dengan pesan
		u2 := h.FrontendBaseURL + "/auth/login?err=" + url.QueryEscape("Akun tidak aktif")
		return c.Redirect(u2, http.StatusTemporaryRedirect)
	}

	// buat JWT sama seperti login biasa
	jwtToken, err := utils.SignJWT(h.JWTSecret, u.ID.String(), string(u.Role), h.Expires)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to sign jwt")
	}

	c.Cookie(&fiber.Cookie{
		Name:     "jm_token",
		Value:    jwtToken,
		Path:     "/",
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		MaxAge:   h.Expires * 60,
	})

	// hapus cookie state
	c.Cookie(&fiber.Cookie{Name: "oauth_state", Value: "", Path: "/", MaxAge: -1, HTTPOnly: true, Secure: false, SameSite: "Lax"})
	c.Cookie(&fiber.Cookie{Name: "oauth_next", Value: "", Path: "/", MaxAge: -1, HTTPOnly: true, Secure: false, SameSite: "Lax"})

	// redirect balik ke FE (cookie sudah ke-set)
	// opsional: tambahkan toast
	redirectURL := h.FrontendBaseURL + next

	// kalau kamu butuh memastikan path next valid:
	// (minimal check: harus diawali '/')
	if !strings.HasPrefix(next, "/") {
		redirectURL = h.FrontendBaseURL + "/"
	}

	// pendek delay tidak perlu, cookie akan ikut response ini
	_ = time.Now()

	return c.Redirect(redirectURL, http.StatusTemporaryRedirect)
}
