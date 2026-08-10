package app

import (
	"context"

	"github.com/fromforgesoftware/aegis/pkg/authn"
	"github.com/fromforgesoftware/go-kit/application/repository"
	"github.com/fromforgesoftware/go-kit/application/usecase"
	apierrors "github.com/fromforgesoftware/go-kit/errors"
	"github.com/fromforgesoftware/go-kit/filter"
	"github.com/fromforgesoftware/go-kit/resource"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/search/query"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

// ConnectionRepository persists connections via kit generics.
type ConnectionRepository interface {
	repository.Creator[domain.Connection]
	repository.Getter[domain.Connection]
	repository.Lister[domain.Connection]
	repository.Deleter
	// SetStatus is the repo's own single-column update, shared with ConnectionStore. Reaching for the
	// generic Patcher instead would mean adding a method the concrete repository does not implement,
	// and fx.As checks that by reflection at startup rather than at compile time.
	SetStatus(ctx context.Context, id string, status domain.ConnectionStatus) error
}

// ConnectionUsecase is the management surface for connections.
//
// EVERY method scopes to the caller's owner, taken from the verified token on the context and never
// from the request. That is enforced here rather than in the transport for two reasons: a filter
// applied to a URL query can be replaced by the client, and there is more than one route reaching
// this usecase — five of them, and the one that gets forgotten is the vulnerability.
//
// This is not defence in depth over an existing check; it is the only check. Until this existed the
// routes were reachable with no credential at all, and `owner` was whatever string the caller put
// in the body: a connection could be created for another workspace, its credential overwritten, or
// the row hard-deleted, by anyone who could reach the ingress.
type ConnectionUsecase interface {
	repository.Getter[domain.Connection]
	repository.Lister[domain.Connection]
	repository.Deleter
	Create(ctx context.Context, conn domain.Connection) (domain.Connection, error)
	// SetStatus enables or disables a connection without destroying its credential, so a UI can
	// offer a reversible toggle. Deleting is the destructive alternative and stays separate.
	SetStatus(ctx context.Context, id string, status domain.ConnectionStatus) (domain.Connection, error)
	// OwnedConnection returns the connection only if it belongs to the caller. The credential and
	// OAuth routes call it before touching secret material.
	OwnedConnection(ctx context.Context, id string) (domain.Connection, error)
}

type connectionUsecase struct {
	usecase.Getter[domain.Connection]
	usecase.Lister[domain.Connection]

	conns    ConnectionRepository
	registry ConnectorRegistry
}

func NewConnectionUsecase(conns ConnectionRepository, registry ConnectorRegistry) ConnectionUsecase {
	return &connectionUsecase{
		Getter:   usecase.NewGetter(conns, domain.ResourceTypeConnection),
		Lister:   usecase.NewLister(conns),
		conns:    conns,
		registry: registry,
	}
}

// ownerOf returns the caller's owner key, or an error when the request was not authenticated.
//
// It fails CLOSED. authn.OwnerFromCtx returns "" for an unauthenticated context, and treating that
// as "no owner filter" would turn a missing token into a request for every workspace's data — which
// is precisely the bug this replaces.
func ownerOf(ctx context.Context) (string, error) {
	owner := authn.OwnerFromCtx(ctx)
	if owner == "" {
		return "", apierrors.Unauthorized("authentication required")
	}
	return owner, nil
}

// List returns only the caller's connections.
//
// The owner filter is appended AFTER the caller's own search options, so a client-supplied
// filter[owner] cannot widen the result: the last filter on a field wins in the kit's query
// builder, and this one is always last.
func (u *connectionUsecase) List(
	ctx context.Context, opts ...search.Option,
) (resource.ListResponse[domain.Connection], error) {
	owner, err := ownerOf(ctx)
	if err != nil {
		// An empty response rather than a nil interface, so a caller that ignores the error does
		// not dereference nil — it sees zero connections, which is also the safe answer.
		return resource.NewEmptyListResponse[domain.Connection](), err
	}
	scoped := append(opts, search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, owner)))
	return u.Lister.List(ctx, scoped...)
}

// Get returns a connection only when the caller owns it.
func (u *connectionUsecase) Get(ctx context.Context, opts ...search.Option) (domain.Connection, error) {
	owner, err := ownerOf(ctx)
	if err != nil {
		return nil, err
	}
	scoped := append(opts, search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, owner)))
	conn, err := u.Getter.Get(ctx, scoped...)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		// NotFound rather than Forbidden: a connection in another workspace should not be confirmed
		// to exist, and the caller was never going to be allowed to read it either way.
		return nil, apierrors.NotFound("connection", "")
	}
	return conn, nil
}

// Delete removes a connection only when the caller owns it.
//
// The kit's generic Deleter takes the same search options as a read, so without the ownership check
// the id alone was enough — and this delete is HARD, taking the sealed credential with it.
func (u *connectionUsecase) Delete(
	ctx context.Context, delType repository.DeleteType, opts ...search.Option,
) error {
	owner, err := ownerOf(ctx)
	if err != nil {
		return err
	}
	scoped := append(opts, search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, owner)))
	// Confirm it exists and is ours first, so deleting someone else's id reports 404 rather than
	// silently succeeding against zero rows.
	existing, err := u.Getter.Get(ctx, scoped...)
	if err != nil {
		return err
	}
	if existing == nil {
		return apierrors.NotFound("connection", "")
	}
	return u.conns.Delete(ctx, delType, scoped...)
}

func (u *connectionUsecase) Create(ctx context.Context, conn domain.Connection) (domain.Connection, error) {
	owner, err := ownerOf(ctx)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, apierrors.InvalidArgument("a connection is required")
	}
	if _, ok := u.registry.Lookup(conn.Connector()); !ok {
		return nil, apierrors.InvalidArgument("unknown connector: " + conn.Connector())
	}
	// The owner is REPLACED with the caller's, never read from the request. A body-supplied owner
	// was how a connection could be created for a workspace the caller had nothing to do with.
	owned := domain.NewConnection(owner, conn.Connector(),
		domain.WithConnectionStatus(conn.Status()),
		domain.WithConnectionScopes(conn.Scopes()),
		domain.WithConnectionExpiresAt(conn.ExpiresAt()),
	)
	return u.conns.Create(ctx, owned)
}

func (u *connectionUsecase) OwnedConnection(ctx context.Context, id string) (domain.Connection, error) {
	if id == "" {
		return nil, apierrors.InvalidArgument("a connection id is required")
	}
	return u.Get(ctx, search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.ID, id)))
}

func (u *connectionUsecase) SetStatus(
	ctx context.Context, id string, status domain.ConnectionStatus,
) (domain.Connection, error) {
	if !status.Valid() {
		return nil, apierrors.InvalidArgument("unknown connection status: " + string(status))
	}
	// Ownership first: the patch below filters by id only, so this is what stops a caller changing
	// another workspace's connection.
	if _, err := u.OwnedConnection(ctx, id); err != nil {
		return nil, err
	}
	if err := u.conns.SetStatus(ctx, id, status); err != nil {
		return nil, err
	}
	// Re-read rather than assembling the result here, so the response reflects what was stored.
	return u.OwnedConnection(ctx, id)
}
