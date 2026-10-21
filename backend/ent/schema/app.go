package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/pkg/entx"
)

// App holds the schema definition for the app entity.
//
// An App is a cross-platform application (Web/Android/iOS/HarmonyOS/desktop)
// authored in the AuthyIDE user center. After the first successful build the
// app is frozen for free-tier users; Pro users can keep editing and rebuilding.
type App struct {
	ent.Schema
}

func (App) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("apps"),
		entx.NewCursor(entx.CursorKindCreatedAt),
	}
}

func (App) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the App.
func (App) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		// name and icon_key are editable in the user center while the app is unlocked.
		field.String("name").NotEmpty(),
		field.String("package_id").NotEmpty(),
		field.String("icon_key").Optional(),
		// ad_network is one of packer/ad_networks.yaml; "none" means the app ships
		// without ads. The ad app id belongs to the end user's own account.
		field.String("ad_network").Default("none"),
		field.String("ad_app_id").Optional(),
		// enabled_platforms is the subset of web/android/ios/harmonyos/desktop
		// that this app targets.
		field.JSON("enabled_platforms", []string{}).Optional(),
		// build_count only increments on a successful build. The free tier allows
		// exactly one successful build; a failed build never consumes the quota.
		field.Int("build_count").Default(0),
		// locked freezes edits to the app in the user center.
		field.Bool("locked").Default(false),
		field.String("status").Default("draft"),
		field.Time("last_built_at").Optional().Nillable(),
		field.String("last_build_platform").Optional(),
		field.String("last_build_artifact_key").Optional(),
		field.Text("description").Optional(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the App.
func (App) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("apps").Field("user_id").Unique().Required(),
	}
}

// Indexes of the App.
func (App) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("user_id", "package_id").Unique(),
	}
}

// Entitlement holds the schema definition for the entitlement entity.
//
// One row per user, describing the tier that governs build quota, editability
// and the optional desktop license.
type Entitlement struct {
	ent.Schema
}

func (Entitlement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("entitlements"),
	}
}

// Fields of the Entitlement.
func (Entitlement) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		// tier is "free" or "pro". Free allows exactly one successful build, then
		// the app locks. Pro removes the build cap and unlocks the desktop license.
		field.String("tier").Default("free"),
		// free_builds_used mirrors the app-level build_count for the whole account.
		field.Int("free_builds_used").Default(0),
		// desktop_licensed gates the paid desktop build target.
		field.Bool("desktop_licensed").Default(false),
		field.Time("pro_expires_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the Entitlement.
func (Entitlement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("entitlements").Field("user_id").Unique().Required(),
	}
}

// Indexes of the Entitlement.
func (Entitlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
	}
}
