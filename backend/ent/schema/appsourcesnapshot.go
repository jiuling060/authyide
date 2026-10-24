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

// AppSourceSnapshot holds the schema definition for a session source snapshot.
//
// One row per "sync source" action in a session. The row captures the state of
// the session workspace at one point in time: a tar.gz of the workspace pushed
// straight to object storage. Apps reference a row to build from that source
// instead of the instance-wide template.
//
// Rows are never soft-deleted: historical build jobs point at object_key for
// provenance, and deleting a row would leave an artifact with no source of
// record. superseded is an audit flag only.
type AppSourceSnapshot struct {
	ent.Schema
}

func (AppSourceSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Table("app_source_snapshots"),
		entx.NewCursor(entx.CursorKindCreatedAt),
	}
}

// Fields of the AppSourceSnapshot.
func (AppSourceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("user_id", uuid.UUID{}),
		// task_id is the session this snapshot was taken from. The session may be
		// recycled after the snapshot is ready; the row keeps its own copy.
		field.UUID("task_id", uuid.UUID{}),
		// object_key is under ObjectStorage.RepoPrefix, so the session host can
		// read its own snapshots back without a new allowlist entry.
		field.String("object_key").NotEmpty(),
		field.String("sha256").NotEmpty(),
		field.Int64("size_bytes"),
		field.Int("file_count"),
		// status is one of uploading/ready/failed.
		field.String("status").Default("uploading"),
		// superseded marks an older snapshot of the same session. It is never
		// used to filter out history: build jobs still reference these rows.
		field.Bool("superseded").Default(false),
		field.Text("error").Optional(),
		field.Time("created_at").Default(time.Now),
		field.Time("completed_at").Optional().Nillable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the AppSourceSnapshot.
func (AppSourceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("task_id"),
		index.Fields("status"),
	}
}
