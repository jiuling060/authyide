package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/pkg/entx"
)

// AppDesktopBranding holds the schema definition for the per-app desktop
// branding entity.
//
// One row per app. The desktop pipeline overlays product_name and
// bundle_identifier onto the Tauri shell, so two user apps packaged in the same
// instance must not share an identifier. The updater endpoint and the icon stay
// out of this table: the endpoint is derived from the app id, and the icon
// already lives in apps.icon_key.
type AppDesktopBranding struct {
	ent.Schema
}

func (AppDesktopBranding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("app_desktop_branding"),
	}
}

func (AppDesktopBranding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the AppDesktopBranding.
func (AppDesktopBranding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		// Required fields are validated in the usecase, which pre-fills them from
		// the app's own name and package id, so the repository may create a
		// partial row.
		field.UUID("app_id", uuid.UUID{}),
		field.String("product_name").Default(""),
		field.String("bundle_identifier").Default(""),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the AppDesktopBranding.
func (AppDesktopBranding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("app_id").Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
