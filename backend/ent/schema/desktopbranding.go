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

// DesktopBranding holds the schema definition for the desktop branding entity.
//
// A separate singleton from app_branding: the desktop and mobile clients evolve
// their identity independently, so one shared row would couple two release
// trains that ship on different cadences.
type DesktopBranding struct {
	ent.Schema
}

func (DesktopBranding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("desktop_branding"),
	}
}

func (DesktopBranding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the DesktopBranding.
func (DesktopBranding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		// singleton_key is constant; its partial unique index keeps one live row.
		field.String("singleton_key").Default("authyide"),
		// Required fields are validated in the usecase, which pre-fills them
		// from tauri.conf.json, so the repository can create a partial row.
		field.String("product_name").Default(""),
		field.String("bundle_identifier").Default(""),
		field.String("updater_endpoint").Default(""),
		field.String("icon_key").Default(""),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the DesktopBranding.
func (DesktopBranding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("singleton_key").Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
