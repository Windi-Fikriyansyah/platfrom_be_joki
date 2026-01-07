package handlers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/models"
	"github.com/Windi-Fikriyansyah/platfrom_be_joki/internal/utils"
)

type FreelancerOnboardingHandler struct {
	DB            *gorm.DB
	UploadDir     string
	PublicBaseURL string
	JWTSecret     string
	ExpiresMin    int
}

func isUniqueViolation(err error) bool {
	// postgres unique violation
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate key value") ||
		strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func NewFreelancerOnboardingHandler(
	db *gorm.DB,
	uploadDir, publicBaseURL string,
	jwtSecret string,
	expiresMin int,
) *FreelancerOnboardingHandler {
	return &FreelancerOnboardingHandler{
		DB:            db,
		UploadDir:     uploadDir,
		PublicBaseURL: publicBaseURL,
		JWTSecret:     jwtSecret,
		ExpiresMin:    expiresMin,
	}
}

func (h *FreelancerOnboardingHandler) Routes(r fiber.Router, authMiddleware fiber.Handler) {
	g := r.Group("/freelancer/onboarding", authMiddleware)
	g.Get("/", h.Get)
	g.Post("/photo", h.UploadPhoto)
	g.Patch("/profile", h.UpdateProfile)
	g.Patch("/about", h.UpdateAbout)
	g.Patch("/identity", h.UpdateIdentity)
	g.Patch("/contact", h.UpdateContact)
	g.Post("/submit", h.Submit)
}

// ========= Helpers =========

func fail200(c *fiber.Ctx, message string, extra ...fiber.Map) error {
	resp := fiber.Map{
		"success": false,
		"message": message,
	}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			resp[k] = v
		}
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

func fail500(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"success": false,
		"message": message,
	})
}

func getAuth(c *fiber.Ctx) (uuid.UUID, error) {
	rawID, ok := c.Locals("userId").(string)
	if !ok || rawID == "" {
		return uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	uID, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "invalid user id")
	}
	return uID, nil
}

func isDigitsLen(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	return phone
}

func validateFreelancerType(t models.FreelancerType) bool {
	return t == models.FreelancerFullTime || t == models.FreelancerPartTime
}

func bumpStep(current int, to int) int {
	if to > current {
		return to
	}
	return current
}

func (h *FreelancerOnboardingHandler) getUserEmail(tx *gorm.DB, userID uuid.UUID) (string, error) {
	var u models.User
	if err := tx.Select("email", "is_active").First(&u, "id = ?", userID).Error; err != nil {
		return "", err
	}
	if !u.IsActive {
		return "", fiber.NewError(fiber.StatusForbidden, "user is inactive")
	}
	return strings.ToLower(strings.TrimSpace(u.Email)), nil
}

func (h *FreelancerOnboardingHandler) findOrCreateProfile(tx *gorm.DB, userID uuid.UUID) (*models.FreelancerProfile, error) {
	var p models.FreelancerProfile
	err := tx.Where("user_id = ?", userID).First(&p).Error

	email, errEmail := h.getUserEmail(tx, userID)
	if errEmail != nil {
		return nil, errEmail
	}

	if err == nil {
		// pastikan contact_email konsisten dengan tabel users
		if p.ContactEmail == "" || p.ContactEmail != email {
			p.ContactEmail = email
			if err := tx.Save(&p).Error; err != nil {
				return nil, err
			}
		}
		return &p, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	p = models.FreelancerProfile{
		UserID:           userID,
		OnboardingStep:   1,
		OnboardingStatus: models.StatusDraft,
		ContactEmail:     email,
	}

	if err := tx.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ========= Handlers =========

func (h *FreelancerOnboardingHandler) Get(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	p, err := h.findOrCreateProfile(h.DB, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}

	return c.JSON(fiber.Map{"success": true, "data": p})
}

// Step 1: upload photo (multipart field: photo)
func (h *FreelancerOnboardingHandler) UploadPhoto(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("photo")
	if err != nil {
		return fail200(c, "photo is required (multipart field: photo)")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return fail200(c, "photo must be jpg/jpeg/png")
	}

	if file.Size > 2*1024*1024 {
		return fail200(c, "photo max size is 2MB")
	}

	dir := filepath.Join(h.UploadDir, "freelancers", userID.String())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create upload dir")
	}

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	dst := filepath.Join(dir, filename)

	if err := c.SaveFile(file, dst); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save file")
	}

	base := strings.TrimRight(h.PublicBaseURL, "/")
	// jika base kosong, tetap simpan path relatif
	publicURL := fmt.Sprintf("%s/uploads/freelancers/%s/%s", base, userID.String(), filename)
	if base == "" {
		publicURL = fmt.Sprintf("/uploads/freelancers/%s/%s", userID.String(), filename)
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}

	p.PhotoURL = publicURL
	p.OnboardingStep = bumpStep(p.OnboardingStep, 1)
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to update profile")
	}

	tx.Commit()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "photo uploaded",
		"data":    p,
	})
}

// Step 2: basic profile
type updateProfileReq struct {
	SystemName     string `json:"system_name"`
	FreelancerType string `json:"freelancer_type"` // full_time / part_time
}

func (h *FreelancerOnboardingHandler) UpdateProfile(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var req updateProfileReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	systemName := strings.TrimSpace(req.SystemName)
	ftype := models.FreelancerType(strings.TrimSpace(req.FreelancerType))

	if systemName == "" {
		return fail200(c, "system_name is required")
	}
	if !validateFreelancerType(ftype) {
		return fail200(c, "freelancer_type must be full_time or part_time")
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}

	if p.PhotoURL == "" {
		tx.Rollback()
		return fail200(c, "complete step 1 (upload photo) first")
	}
	if p.OnboardingStatus != models.StatusDraft {
		tx.Rollback()
		return fail200(c, "onboarding already submitted/reviewed")
	}

	p.SystemName = systemName
	p.FreelancerType = ftype
	p.OnboardingStep = bumpStep(p.OnboardingStep, 2)
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to update")
	}

	tx.Commit()

	return c.JSON(fiber.Map{"success": true, "data": p})
}

// Step 3: about
type updateAboutReq struct {
	About string `json:"about"`
}

func (h *FreelancerOnboardingHandler) UpdateAbout(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var req updateAboutReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	about := strings.TrimSpace(req.About)
	if len(about) < 10 {
		return fiber.NewError(fiber.StatusBadRequest, "about minimal 10 karakter")
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}

	if p.SystemName == "" || !validateFreelancerType(p.FreelancerType) {
		tx.Rollback()
		return fiber.NewError(fiber.StatusBadRequest, "complete step 2 first")
	}
	if p.OnboardingStatus != models.StatusDraft {
		tx.Rollback()
		return fiber.NewError(fiber.StatusBadRequest, "onboarding already submitted/reviewed")
	}

	p.About = about
	p.OnboardingStep = bumpStep(p.OnboardingStep, 3)
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to update")
	}

	tx.Commit()

	return c.JSON(fiber.Map{"success": true, "data": p})
}

// Step 4: identity (KTP)
type updateIdentityReq struct {
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	LastName   string `json:"last_name"`
	NIK        string `json:"nik"`
	KTPAddress string `json:"ktp_address"`
	PostalCode string `json:"postal_code"`
	Kelurahan  string `json:"kelurahan"`
	Kecamatan  string `json:"kecamatan"`
	City       string `json:"city"`
}

var postalRe = regexp.MustCompile(`^\d{5}$`)

func (h *FreelancerOnboardingHandler) UpdateIdentity(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err // 401 tetap 401
	}

	var req updateIdentityReq
	if err := c.BodyParser(&req); err != nil {
		return fail200(c, "invalid body")
	}

	fn := strings.TrimSpace(req.FirstName)
	ln := strings.TrimSpace(req.LastName)
	mn := strings.TrimSpace(req.MiddleName)
	nik := strings.TrimSpace(req.NIK)
	ktpAddr := strings.TrimSpace(req.KTPAddress)

	postal := strings.TrimSpace(req.PostalCode)
	kel := strings.TrimSpace(req.Kelurahan)
	kec := strings.TrimSpace(req.Kecamatan)
	city := strings.TrimSpace(req.City)

	// ✅ Validasi -> fail200
	if fn == "" || ln == "" {
		return fail200(c, "first_name and last_name are required")
	}
	if !isDigitsLen(nik, 16) {
		return fail200(c, "nik must be 16 digit angka")
	}
	if ktpAddr == "" {
		return fail200(c, "ktp_address is required")
	}
	if postal != "" && !postalRe.MatchString(postal) {
		return fail200(c, "postal_code must be 5 digit")
	}
	if kel == "" || kec == "" || city == "" {
		return fail200(c, "kelurahan, kecamatan, city are required")
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fail500(c, "failed to load profile")
	}

	// ✅ Validasi step & status -> fail200
	if p.About == "" {
		tx.Rollback()
		return fail200(c, "complete step 3 first")
	}
	if p.OnboardingStatus != models.StatusDraft {
		tx.Rollback()
		return fail200(c, "onboarding already submitted/reviewed")
	}

	// cek NIK sudah dipakai user lain
	var count int64
	if err := tx.Model(&models.FreelancerProfile{}).
		Where("nik = ? AND user_id <> ?", nik, userID).
		Count(&count).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to validate nik")
	}
	if count > 0 {
		tx.Rollback()
		return fail200(c, "nik sudah terdaftar")
	}

	// update
	p.FirstName = fn
	p.MiddleName = mn
	p.LastName = ln
	p.NIK = nik
	p.KTPAddress = ktpAddr
	p.PostalCode = postal
	p.Kelurahan = kel
	p.Kecamatan = kec
	p.City = city
	p.OnboardingStep = bumpStep(p.OnboardingStep, 4)
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		if isUniqueViolation(err) {
			return fail200(c, "nik sudah terdaftar")
		}
		return fail500(c, "failed to update")
	}

	tx.Commit()
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "data": p})
}

// Step 5: contact confirm
type updateContactReq struct {
	ContactPhone   string `json:"contact_phone"`
	CurrentAddress string `json:"current_address"`
}

func (h *FreelancerOnboardingHandler) UpdateContact(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	var req updateContactReq
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	phone := normalizePhone(req.ContactPhone)
	addr := strings.TrimSpace(req.CurrentAddress)

	if phone == "" {
		return fiber.NewError(fiber.StatusBadRequest, "contact_phone is required")
	}
	if len(phone) < 9 {
		return fiber.NewError(fiber.StatusBadRequest, "contact_phone terlalu pendek")
	}
	if addr == "" {
		return fiber.NewError(fiber.StatusBadRequest, "current_address is required")
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}

	if p.NIK == "" || p.KTPAddress == "" || p.City == "" {
		tx.Rollback()
		return fiber.NewError(fiber.StatusBadRequest, "complete step 4 first")
	}
	if p.OnboardingStatus != models.StatusDraft {
		tx.Rollback()
		return fiber.NewError(fiber.StatusBadRequest, "onboarding already submitted/reviewed")
	}

	// contact_email selalu dari users (sudah dijamin oleh findOrCreateProfile)
	p.ContactPhone = phone
	p.CurrentAddress = addr

	p.OnboardingStep = bumpStep(p.OnboardingStep, 5)
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update")
	}
	tx.Commit()

	return c.JSON(fiber.Map{"success": true, "data": p})
}

// Final submit
func (h *FreelancerOnboardingHandler) Submit(c *fiber.Ctx) error {
	userID, err := getAuth(c)
	if err != nil {
		return err
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	p, err := h.findOrCreateProfile(tx, userID)
	if err != nil {
		tx.Rollback()
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load profile")
	}
	if p.OnboardingStatus != models.StatusDraft {
		tx.Rollback()
		return fiber.NewError(fiber.StatusBadRequest, "already submitted/reviewed")
	}

	missing := []string{}
	if p.PhotoURL == "" {
		missing = append(missing, "photo")
	}
	if p.SystemName == "" {
		missing = append(missing, "system_name")
	}
	if !validateFreelancerType(p.FreelancerType) {
		missing = append(missing, "freelancer_type")
	}
	if p.About == "" {
		missing = append(missing, "about")
	}
	if p.FirstName == "" {
		missing = append(missing, "first_name")
	}
	if p.LastName == "" {
		missing = append(missing, "last_name")
	}
	if !isDigitsLen(p.NIK, 16) {
		missing = append(missing, "nik")
	}
	if p.KTPAddress == "" {
		missing = append(missing, "ktp_address")
	}
	if p.Kelurahan == "" {
		missing = append(missing, "kelurahan")
	}
	if p.Kecamatan == "" {
		missing = append(missing, "kecamatan")
	}
	if p.City == "" {
		missing = append(missing, "city")
	}
	if p.ContactEmail == "" {
		missing = append(missing, "contact_email")
	}
	if p.ContactPhone == "" {
		missing = append(missing, "contact_phone")
	}
	if p.CurrentAddress == "" {
		missing = append(missing, "current_address")
	}

	if len(missing) > 0 {
		tx.Rollback()
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"success": false,
			"message": "data belum lengkap",
			"missing": missing,
		})
	}

	// ✅ Langsung APPROVED
	p.OnboardingStatus = models.StatusApproved
	p.OnboardingStep = 5
	p.UpdatedAt = time.Now()

	if err := tx.Save(p).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to submit")
	}

	// ✅ Update role user jadi freelancer
	var u models.User
	if err := tx.First(&u, "id = ?", userID).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to load user")
	}

	u.Role = models.RoleFreelancer
	u.UpdatedAt = time.Now()

	if err := tx.Save(&u).Error; err != nil {
		tx.Rollback()
		return fail500(c, "failed to update user role")
	}

	if err := tx.Commit().Error; err != nil {
		return fail500(c, "failed to commit")
	}

	claims := &utils.Claims{
		UserID: u.ID.String(),
		Role:   string(u.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(
				time.Now().Add(time.Duration(h.ExpiresMin) * time.Minute),
			),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(h.JWTSecret))
	if err != nil {
		// kalau gagal bikin token, user tetap sudah freelancer di DB,
		// tapi mending kasih info error jelas.
		return fail500(c, "failed to issue new token")
	}

	// set cookie baru (samakan opsi dengan AuthHandler.Login)
	c.Cookie(&fiber.Cookie{
		Name:     "jm_token",
		Value:    signed,
		Path:     "/",
		HTTPOnly: true,
		Secure:   false, // kalau production pakai true + HTTPS
		SameSite: "Lax",
		MaxAge:   h.ExpiresMin * 60, // dalam detik
	})

	return c.JSON(fiber.Map{
		"success": true,
		"message": "onboarding approved, role updated to freelancer",
		"data": fiber.Map{
			"profile": p,
			"user":    u,
		},
	})
}

// Public Profile Handler
func (h *FreelancerOnboardingHandler) GetPublicProfile(c *fiber.Ctx) error {
	idParam := c.Params("id")
	targetUserID, err := uuid.Parse(idParam)
	if err != nil {
		return fail200(c, "invalid user id")
	}

	// 1. Get Profile
	var profile models.FreelancerProfile
	if err := h.DB.Where("user_id = ?", targetUserID).First(&profile).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"success": false,
			"message": "Profil tidak ditemukan",
		})
	}

	// 2. Get User Info (Joined At)
	var user models.User
	h.DB.Select("created_at").First(&user, "id = ?", targetUserID)

	// 3. Get Products (Published Only) with aggregated stats
	type ProductResult struct {
		ID          uint
		Title       string
		Category    string
		BasePrice   int64
		CoverURL    string
		AvgRating   float64
		ReviewCount int64
	}

	var products []ProductResult
	h.DB.Table("products").
		Select(`
			products.id,
			products.title,
			products.category,
			products.base_price,
			products.cover_url,
			(SELECT COALESCE(AVG(rating), 0) FROM reviews r WHERE r.product_id = products.id) as avg_rating,
			(SELECT COUNT(*) FROM reviews r WHERE r.product_id = products.id) as review_count
		`).
		Where("products.user_id = ? AND products.status = ?", targetUserID, "published").
		Order("products.created_at DESC").
		Scan(&products)

	// Format products output
	outProducts := make([]fiber.Map, 0, len(products))
	for _, p := range products {
		encID, _ := utils.EncryptID(p.ID, os.Getenv("ID_ENCRYPT_KEY"))
		outProducts = append(outProducts, fiber.Map{
			"id":           encID,
			"title":        p.Title,
			"category":     p.Category,
			"price":        p.BasePrice,
			"cover":        p.CoverURL,
			"rating":       p.AvgRating,
			"review_count": p.ReviewCount,
		})
	}

	// 4. Get All Reviews for this freelancer (via products)
	// We can join reviews -> products -> user_id
	var reviews []models.Review
	h.DB.Table("reviews").
		Joins("JOIN products ON products.id = reviews.product_id").
		Preload("Client"). // Load reviewer info
		Where("products.user_id = ?", targetUserID).
		Order("reviews.created_at DESC").
		Limit(20). // Limit recent reviews
		Find(&reviews)

	// Calculate total stats
	var totalStats struct {
		AvgRating   float64
		ReviewCount int64
	}
	h.DB.Table("reviews").
		Joins("JOIN products ON products.id = reviews.product_id").
		Where("products.user_id = ?", targetUserID).
		Select("COALESCE(AVG(reviews.rating), 0) as avg_rating, COUNT(reviews.id) as review_count").
		Scan(&totalStats)

	outReviews := make([]fiber.Map, 0, len(reviews))
	for _, r := range reviews {
		reviewerName := "Pengguna"
		if r.Client != nil {
			reviewerName = r.Client.Name
		}
		outReviews = append(outReviews, fiber.Map{
			"id":         r.ID,
			"rating":     r.Rating,
			"comment":    r.Comment,
			"created_at": r.CreatedAt,
			"reviewer": fiber.Map{
				"name": reviewerName,
			},
		})
	}

	// Determine freelancer level display
	level := "Freelancer"
	switch profile.FreelancerType {
	case models.FreelancerFullTime:
		level = "Full Time"
	case models.FreelancerPartTime:
		level = "Part Time"
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"profile": fiber.Map{
				"id":           profile.UserID,
				"name":         profile.SystemName,
				"title":        "Freelancer", // Or specific title if available
				"level":        level,
				"photo_url":    profile.PhotoURL,
				"about":        profile.About,
				"location":     fmt.Sprintf("%s, %s", profile.City, profile.Kecamatan), // Simple location
				"joined_at":    user.CreatedAt,
				"rating":       totalStats.AvgRating,
				"review_count": totalStats.ReviewCount,
			},
			"products": outProducts,
			"reviews":  outReviews,
		},
	})
}
