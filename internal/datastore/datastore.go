package datastore

import (
	"context"

	v0 "github.com/authzed/authzed-go/proto/authzed/api/v0"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/shopspring/decimal"
)

// DataSource is the base interface that both namespace and relationship data
// sources must implement.
type DataSource interface {
	// IsReady returns whether the datastore is ready to accept data. Datastores that require
	// database schema creation will return false until the migrations have been run to create
	// the necessary tables.
	IsReady(ctx context.Context) (bool, error)

	// Close closes the data store.
	Close() error
}

// Relationships represents relationship access.
type Relationships interface {
	DataSource

	// QuantizedRevision gets a revision that will likely already be replicated
	// and will likely be shared amongst many queries.
	QuantizedRevision(ctx context.Context) (Revision, error)

	// HeadRevision gets a revision that is guaranteed to be at least as fresh as
	// right now.
	HeadRevision(ctx context.Context) (Revision, error)

	// BeginReadOnly creates a transaction which can be used for read-only
	// relationship operations.
	BeginReadOnly(ctx context.Context, revision Revision) ReadOnlyTransaction

	// Begin creates a transaction which can be used for read-write relationship
	// operations. The specified revision is used for read/precondition operations
	// and the new revision created by mutations is returned by the Commit operation.
	Begin(ctx context.Context, revision Revision) Transaction

	// Watch notifies the caller about all changes to relationships.
	//
	// All events following afterRevision will be sent to the caller.
	Watch(ctx context.Context, afterRevision Revision) (<-chan *RevisionChanges, <-chan error)
}

// Namespaces is an interface for persisting and loading namespace definitions.
type Namespaces interface {
	DataSource

	// WriteNamespace takes a proto namespace definition and persists it,
	// returning the version of the namespace that was created.
	WriteNamespace(ctx context.Context, newConfig *v0.NamespaceDefinition) (Revision, error)

	// ReadNamespace reads a namespace definition and version and returns it if
	// found.
	ReadNamespace(ctx context.Context, nsName string) (*v0.NamespaceDefinition, Revision, error)

	// DeleteNamespace deletes a namespace and any associated relationships.
	DeleteNamespace(ctx context.Context, nsName string) (Revision, error)

	// ListNamespaces lists all namespaces defined.
	ListNamespaces(ctx context.Context) ([]*v0.NamespaceDefinition, error)
}

// Transaction is used for performing multiple operations on a datastore with
// a single connection and the ability to rollback.
type Transaction interface {
	ReadOnlyTransaction

	// WriteRelationships takes a list of tuple mutations and applies them to the datastore.
	WriteRelationships(ctx context.Context, mutations []*v1.RelationshipUpdate) error

	// DeleteRelationships deletes all Relationships that match the provided filter.
	DeleteRelationships(ctx context.Context, filter *v1.RelationshipFilter) error

	// Commit commits the underlying transaction and returns the datastore revision
	// at which the transaction was committed.
	Commit(ctx context.Context) (Revision, error)
}

// ReadOnlyTransaction is used performing read-only operations at a specified
// snapshot.
type ReadOnlyTransaction interface {
	// QueryRelationships creates a builder for reading relationships from the datastore.
	QueryRelationships(resourceFilter RelationshipQueryObjectFilter) RelationshipQuery

	// ReverseQueryRelationships creates a builder for reading relationships from the subject.
	ReverseQueryRelationships(resourceFilter RelationshipQueryObjectFilter) ReverseRelationshipQuery

	// CheckPreconditions verifies that the existing preconditions are met at
	// the transaction revision, or returns an ErrPreconditionFailed.
	CheckPreconditions(ctx context.Context, preconditions []*v1.Precondition) error

	// Rollback abandons the current transaction in a way that underlying mutations
	// are not applied.
	Rollback(ctx context.Context) error
}

// RelationshipQueryObjectFilter are the baseline fields used to filter results when
// querying a datastore for relationships.
//
// OptionalFields are ignored when their value is the empty string.
type RelationshipQueryObjectFilter struct {
	ResourceType             string
	OptionalResourceID       string
	OptionalResourceRelation string
}

// CommonRelationshipQuery is the common interface shared between RelationshipQuery and
// ReverseRelationshipQuery.
type CommonRelationshipQuery interface {
	// Execute runs the tuple query and returns a result iterator.
	Execute(ctx context.Context) (RelationshipIterator, error)

	// Limit sets a limit on the query.
	Limit(limit uint64) CommonRelationshipQuery
}

// RelationshipQuery is a builder for constructing tuple queries.
type RelationshipQuery interface {
	CommonRelationshipQuery

	// WithSubjectFilter adds a subject filter to the query.
	WithSubjectFilter(*v1.SubjectFilter) RelationshipQuery

	// WithUsersets adds multiple userset filters to the query.
	WithUsersets(usersets []*v1.SubjectReference) RelationshipQuery
}

// ReverseRelationshipQuery is a builder for constructing reverse tuple queries.
type ReverseRelationshipQuery interface {
	CommonRelationshipQuery

	// WithObjectRelation filters to relationships with the given object relation on the
	// left hand side.
	WithObjectRelation(namespace string, relation string) ReverseRelationshipQuery
}

// RelationshipIterator is an iterator over matched relationships.
type RelationshipIterator interface {
	// Next returns the next relationship in the result set.
	Next() *v1.Relationship

	// After receiving a nil response, the caller must check for an error.
	Err() error

	// Close cancels the query and closes any open connections.
	Close()
}

// Revision is a type alias to make changing the revision type a little bit
// easier if we need to do it in the future. Implementations should code
// directly against decimal.Decimal when creating or parsing.
type Revision = decimal.Decimal

// NoRevision is a zero type for the revision that will make changing the
// revision type in the future a bit easier if necessary. Implementations
// should use any time they want to signal an empty/error revision.
var NoRevision Revision

// Ellipsis is a special relation that is assumed to be valid on the right
// hand side of a relationship when the SubjectReference.OptionalRelation is left
// blank.
const Ellipsis = "..."

// RevisionChanges represents the changes in a single transaction.
type RevisionChanges struct {
	Revision Revision
	Changes  []*v1.RelationshipUpdate
}
