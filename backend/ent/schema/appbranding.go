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

// AppBranding holds the schema definition for the mobile app branding entity.
//
// The table holds a single live row: the branding of the one mobile app this
// deployment ships. The partial unique index on singleton_key enforces that
// exactly one live row exists.
type AppBranding struct {
	ent.Schema
}

func (AppBranding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("app_branding"),
	}
}

func (AppBranding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the AppBranding.
func (AppBranding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		// singleton_key is constant; its partial unique index keeps one live row.
		field.String("singleton_key").Default("authyide"),
		// Required fields are validated in the usecase, which pre-fills them from
		// app.json, so the repository can create a partially filled row.
		field.String("name").Default(""),
		field.String("version").Default(""),
		field.Int("version_code").Default(1),
		field.String("server_url").Default(""),
		field.String("updates_server").Default(""),
		field.String("logo_light_key").Default(""),
		field.String("logo_dark_key").Default(""),
		field.String("icon_key").Default(""),
		field.String("icon_dark_key").Default(""),
		field.String("adaptive_icon_key").Default(""),
		field.String("splash_key").Default(""),
		field.String("splash_dark_key").Default(""),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the AppBranding.
func (AppBranding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("singleton_key").Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
