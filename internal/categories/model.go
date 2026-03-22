package categories

import "time"

// CategoryType restricts a category to 'income' or 'expense'.
// Transfer transactions do not have a category.
type CategoryType string

const (
	CategoryTypeIncome  CategoryType = "income"
	CategoryTypeExpense CategoryType = "expense"
)

// IsValid returns true when the value is one of the allowed enum values.
func (t CategoryType) IsValid() bool {
	return t == CategoryTypeIncome || t == CategoryTypeExpense
}

// Category is the GORM model that mirrors the categories table.
//
// Rules:
//   - When UserID is nil the row is a system category (seeded by migration).
//   - IsSystem == true means the row was seeded; users cannot delete or modify it.
//   - User-created categories have UserID set and IsSystem == false.
type Category struct {
	ID        uint         `gorm:"primaryKey"             json:"id"`
	UserID    *uint        `gorm:"index"                  json:"user_id"`   // nil for system rows
	Name      string       `gorm:"size:100;not null"      json:"name"`
	Type      CategoryType `gorm:"size:20;not null"       json:"type"`
	Icon      string       `gorm:"size:50"                json:"icon,omitempty"`
	Color     string       `gorm:"size:20"                json:"color,omitempty"`
	IsSystem  bool         `gorm:"not null;default:false" json:"is_system"`
	CreatedAt time.Time    `                              json:"created_at"`
	UpdatedAt time.Time    `                              json:"updated_at"`
}

// TableName overrides the default GORM table name.
func (Category) TableName() string { return "categories" }

// ─────────────────────────────────────────────────────────────────────────────
// Request / Response types
// ─────────────────────────────────────────────────────────────────────────────

// CreateCategoryRequest carries the fields required to create a user category.
type CreateCategoryRequest struct {
	// Name of the category (required, max 100 chars).
	Name string `json:"name" binding:"required,max=100"`
	// Type must be "income" or "expense".
	Type CategoryType `json:"type" binding:"required"`
	// Icon is an optional emoji or icon identifier (e.g. "🍔", "fa-utensils").
	Icon string `json:"icon"`
	// Color is an optional hex or CSS color (e.g. "#FF5733").
	Color string `json:"color"`
}

// UpdateCategoryRequest carries the fields that may be changed on a user category.
// All fields are optional — only non-zero values are applied.
// Type and ownership cannot be changed after creation.
type UpdateCategoryRequest struct {
	// Name replaces the existing category name (optional, max 100 chars).
	Name string `json:"name" binding:"omitempty,max=100"`
	// Icon replaces the icon (optional).
	Icon string `json:"icon"`
	// Color replaces the color (optional).
	Color string `json:"color"`
}

// CategoryResponse is the JSON shape returned to the client for a single category.
type CategoryResponse struct {
	ID        uint         `json:"id"`
	UserID    *uint        `json:"user_id"`
	Name      string       `json:"name"`
	Type      CategoryType `json:"type"`
	Icon      string       `json:"icon,omitempty"`
	Color     string       `json:"color,omitempty"`
	IsSystem  bool         `json:"is_system"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// CategoryListResponse is the JSON shape returned for a list of categories.
type CategoryListResponse struct {
	Categories []CategoryResponse `json:"categories"`
	Total      int                `json:"total"`
}

// toCategoryResponse converts a Category model to its API response shape.
func toCategoryResponse(c Category) CategoryResponse {
	return CategoryResponse{
		ID:        c.ID,
		UserID:    c.UserID,
		Name:      c.Name,
		Type:      c.Type,
		Icon:      c.Icon,
		Color:     c.Color,
		IsSystem:  c.IsSystem,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// toCategoryResponseList converts a slice of Category models to response shapes.
func toCategoryResponseList(cats []Category) []CategoryResponse {
	out := make([]CategoryResponse, len(cats))
	for i, c := range cats {
		out[i] = toCategoryResponse(c)
	}
	return out
}
