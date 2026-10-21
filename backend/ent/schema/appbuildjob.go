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

// AppBuildJob holds the schema definition for the mobile app build job entity.
//
// One row per packaging request. The worker runs one job at a time, so at most
// one row is ever in status "running".
type AppBuildJob struct {
	ent.Schema
}

func (AppBuildJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("app_build_jobs"),
		entx.NewCursor(entx.CursorKindCreatedAt),
	}
}

func (AppBuildJob) Mixin() []ent.Mixin {
	return []ent.Mixin{
		entx.SoftDeleteMixin2{},
	}
}

// Fields of the AppBuildJob.
func (AppBuildJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		// app_id is nullable: admin-console builds are instance-wide and
		// reference no user-center app. Self-service packaging sets it.
		field.UUID("app_id", uuid.UUID{}).Optional().Nillable(),
		field.String("platform").NotEmpty(),
		// target_version is the expo version string baked into app.json.
		field.String("target_version").NotEmpty(),
		field.Int("version_code").Default(1),
		// status is one of queued/running/succeeded/failed.
		field.String("status").Default("queued"),
		// current_step is one of inject-assets/rewrite-config/prebuild/assemble/verify.
		field.String("current_step").Optional(),
		field.String("worker_id").Optional(),
		// log_path points at the full build log; log_tail is only a view.
		field.Text("log_path").Optional(),
		field.Text("log_tail").Optional(),
		field.Text("error").Optional(),
		field.Int("exit_code").Optional().Nillable(),
		field.Text("artifact_path").Optional(),
		field.Text("artifact_sha256").Optional(),
		field.Int64("artifact_size").Default(0),
		field.Int64("duration_ms").Default(0),
		field.Time("published_at").Optional().Nillable(),
		field.Time("enqueued_at").Default(time.Now),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the AppBuildJob.
func (AppBuildJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "enqueued_at"),
		index.Fields("user_id"),
		index.Fields("app_id", "status"),
		index.Fields("user_id", "enqueued_at"),
		index.Fields("created_at"),
		index.Fields("platform"),
	}
}
