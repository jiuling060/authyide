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

// AppBuildArtifact holds one distributable file of a build job, per channel.
//
// A desktop job emits three rows (deb / rpm / appimage) while a mobile job
// emits a single apk row, so download and publish resolve through this table
// for both platforms instead of special-casing the mobile path.
type AppBuildArtifact struct {
	ent.Schema
}

func (AppBuildArtifact) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("app_build_artifacts"),
	}
}

func (AppBuildArtifact) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the AppBuildArtifact.
func (AppBuildArtifact) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		// job_id is a plain column rather than an ent edge so the schema mirrors
		// the hand-written migration and leaves app_build_jobs untouched.
		field.UUID("job_id", uuid.UUID{}),
		field.String("platform").NotEmpty(),
		field.String("channel").NotEmpty(),
		field.Text("file_name").NotEmpty(),
		field.Text("artifact_path").NotEmpty(),
		field.Text("artifact_sha256").NotEmpty(),
		field.Int64("artifact_size").Default(0),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the AppBuildArtifact.
func (AppBuildArtifact) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id", "channel").Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("platform", "channel"),
	}
}
