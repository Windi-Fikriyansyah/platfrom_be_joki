package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Product struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	Title     string `json:"title"`     
	Category  string `json:"category"`  // Kategori (Skripsi, Makalah, dll)
	BasePrice int64  `json:"base_price"` // Harga awal (dari "Harga Awal" di main form)

	VisibilityDescription string         `json:"visibility_description"` // deskripsi di view "Meningkatkan Visibilitas Produk"
	CoverURL              string         `json:"cover_url"`              // nanti bisa diisi path file cover kalau sudah ada upload
	CoverTransform        datatypes.JSON `json:"cover_transform"`        // simpan transform (scale, pos, flip)

	// Paket & Portofolio disimpan sebagai JSON biar fleksibel dulu
	Packages  datatypes.JSON `json:"packages"`  // { basic: {...}, standard: {...}, premium: {...} }
	Portfolio datatypes.JSON `json:"portfolio"` // { video_url: "...", images: [...] }

	Status string `gorm:"type:varchar(20);default:'draft'" json:"status"` // draft | review | published, dll

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
