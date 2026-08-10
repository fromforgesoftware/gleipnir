package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fromforgesoftware/aegis/pkg/authn"
	"github.com/fromforgesoftware/go-kit/application/repository"
	apierrors "github.com/fromforgesoftware/go-kit/errors"
	"github.com/fromforgesoftware/go-kit/filter"
	"github.com/fromforgesoftware/go-kit/resource"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/search/query"

	"github.com/fromforgesoftware/gleipnir/internal/app"
	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

// This file exists because none of it was tested, and the absence is why the service shipped with
// every connection route open. Verified against a running deployment before the fix: an anonymous
// POST created a connection for an owner of the caller's choosing (201), an anonymous credential
// write was accepted (201), and an anonymous DELETE hard-deleted the row (204).
//
// So the assertions below are mostly about refusal, and about WHICH owner reaches the repository —
// never the one in the request.

const (
	callerOrg = "org-caller"
	otherOrg  = "org-other"
	connID    = "conn-1"
)

// authed returns a context carrying verified claims, as the auth middleware would leave it.
func authed(org string) context.Context {
	return authn.WithClaims(context.Background(), authn.Claims{Subject: "acct-1", OrgID: org})
}

// --- fakes ---

// fakeConnRepo records the search options it is handed, so a test can prove the owner filter
// reached the repository rather than trusting that the usecase built one.
type fakeConnRepo struct {
	rows []domain.Connection

	created   domain.Connection
	createErr error

	deleteCalled bool
	deleteOpts   []search.Option

	patchOpts []repository.PatchOption

	lastGetOpts  []search.Option
	lastListOpts []search.Option
}

func (f *fakeConnRepo) Create(_ context.Context, c domain.Connection) (domain.Connection, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = c
	return c, nil
}

// Get honours the owner filter, because a fake that ignores it would let a broken usecase pass.
func (f *fakeConnRepo) Get(_ context.Context, opts ...search.Option) (domain.Connection, error) {
	f.lastGetOpts = opts
	wantOwner := ownerFilter(opts)
	for _, row := range f.rows {
		if wantOwner != "" && row.Owner() != wantOwner {
			continue
		}
		return row, nil
	}
	return nil, nil
}

func (f *fakeConnRepo) List(
	_ context.Context, opts ...search.Option,
) (resource.ListResponse[domain.Connection], error) {
	f.lastListOpts = opts
	wantOwner := ownerFilter(opts)
	kept := make([]domain.Connection, 0, len(f.rows))
	for _, row := range f.rows {
		if wantOwner == "" || row.Owner() == wantOwner {
			kept = append(kept, row)
		}
	}
	return resource.NewListResponse(kept, len(kept)), nil
}

func (f *fakeConnRepo) Delete(_ context.Context, _ repository.DeleteType, opts ...search.Option) error {
	f.deleteCalled = true
	f.deleteOpts = opts
	return nil
}

func (f *fakeConnRepo) Patch(
	_ context.Context, opts ...repository.PatchOption,
) ([]domain.Connection, error) {
	f.patchOpts = opts
	return f.rows, nil
}

// ownerFilter reads the owner the usecase filtered by out of the search options.
func ownerFilter(opts []search.Option) string {
	q := search.New(opts...).Query()
	if !q.Filters().Exists(fields.Owner) {
		return ""
	}
	return query.GetFilterVal[string](fields.Owner, q.Filters())
}

type fakeConnRegistry struct{ known bool }

func (f *fakeConnRegistry) Lookup(slug string) (domain.Connector, bool) {
	if !f.known {
		return domain.Connector{}, false
	}
	return domain.Connector{Slug: slug, Name: slug, AuthType: domain.AuthTypeAPIKey}, true
}

func (f *fakeConnRegistry) List() []domain.Connector { return nil }

func newUsecase(rows ...domain.Connection) (app.ConnectionUsecase, *fakeConnRepo) {
	repo := &fakeConnRepo{rows: rows}
	return app.NewConnectionUsecase(repo, &fakeConnRegistry{known: true}), repo
}

func connFor(owner string) domain.Connection {
	return domain.NewConnection(owner, "kraken")
}

// --- unauthenticated: every method must fail closed ---

func TestConnectionUsecase_RefusesAnUnauthenticatedContext(t *testing.T) {
	// A context with no claims is what an unauthenticated request produces. OwnerFromCtx returns ""
	// for it, and treating "" as "no filter" is what turned a missing token into a request for every
	// workspace's connections.
	uc, repo := newUsecase(connFor(callerOrg))
	ctx := context.Background()

	t.Run("List", func(t *testing.T) {
		_, err := uc.List(ctx)
		requireUnauthorized(t, err)
	})
	t.Run("Get", func(t *testing.T) {
		_, err := uc.Get(ctx)
		requireUnauthorized(t, err)
	})
	t.Run("Create", func(t *testing.T) {
		_, err := uc.Create(ctx, connFor(callerOrg))
		requireUnauthorized(t, err)
		assert.Nil(t, repo.created, "an unauthenticated create must not reach the repository")
	})
	t.Run("Delete", func(t *testing.T) {
		err := uc.Delete(ctx, repository.DeleteTypeHard)
		requireUnauthorized(t, err)
		assert.False(t, repo.deleteCalled, "an unauthenticated delete must not reach the repository")
	})
	t.Run("SetStatus", func(t *testing.T) {
		_, err := uc.SetStatus(ctx, connID, domain.ConnectionStatusRevoked)
		requireUnauthorized(t, err)
		assert.Nil(t, repo.patchOpts, "an unauthenticated patch must not reach the repository")
	})
	t.Run("OwnedConnection", func(t *testing.T) {
		_, err := uc.OwnedConnection(ctx, connID)
		requireUnauthorized(t, err)
	})
}

func requireUnauthorized(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	apiErr, ok := apierrors.As(err)
	require.True(t, ok, "want a kit error carrying a status, got %#v", err)
	assert.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus())
}

// --- the owner is the caller's, never the request's ---

func TestConnectionUsecase_CreateIgnoresTheRequestedOwner(t *testing.T) {
	uc, repo := newUsecase()

	// The body names a different workspace. This is exactly the request that succeeded against the
	// live deployment, and the created row must not honour it.
	_, err := uc.Create(authed(callerOrg), domain.NewConnection(otherOrg, "kraken"))
	require.NoError(t, err)

	require.NotNil(t, repo.created)
	assert.Equal(t, callerOrg, repo.created.Owner(),
		"the owner must come from the token, not the payload")
	assert.Equal(t, "kraken", repo.created.Connector(), "the connector still comes from the request")
}

func TestConnectionUsecase_CreateRejectsAnUnknownConnector(t *testing.T) {
	repo := &fakeConnRepo{}
	uc := app.NewConnectionUsecase(repo, &fakeConnRegistry{known: false})

	_, err := uc.Create(authed(callerOrg), connFor(callerOrg))
	require.Error(t, err)
	assert.Nil(t, repo.created)
}

func TestConnectionUsecase_ListIsScopedToTheCaller(t *testing.T) {
	uc, repo := newUsecase(connFor(callerOrg), connFor(otherOrg), connFor(callerOrg))

	got, err := uc.List(authed(callerOrg))
	require.NoError(t, err)

	assert.Equal(t, callerOrg, ownerFilter(repo.lastListOpts),
		"the repository must be asked for the caller's connections only")
	assert.Len(t, got.Results(), 2)
	for _, row := range got.Results() {
		assert.Equal(t, callerOrg, row.Owner())
	}
}

func TestConnectionUsecase_ListCannotBeWidenedByTheCaller(t *testing.T) {
	// A client-supplied filter[owner] arrives as a search option. The usecase appends its own
	// afterwards, so the caller's own scope wins — otherwise the scoping would be advisory.
	uc, repo := newUsecase(connFor(callerOrg), connFor(otherOrg))

	_, err := uc.List(authed(callerOrg),
		search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, otherOrg)))
	require.NoError(t, err)

	assert.Equal(t, callerOrg, ownerFilter(repo.lastListOpts),
		"a request-supplied owner filter must not survive")
}

func TestConnectionUsecase_GetHidesAnotherWorkspacesConnection(t *testing.T) {
	uc, _ := newUsecase(connFor(otherOrg))

	_, err := uc.Get(authed(callerOrg))
	require.Error(t, err)
	apiErr, ok := apierrors.As(err)
	require.True(t, ok)
	// 404, not 403: a connection in another workspace should not be confirmed to exist.
	assert.Equal(t, http.StatusNotFound, apiErr.HTTPStatus())
}

func TestConnectionUsecase_DeleteRefusesAnotherWorkspacesConnection(t *testing.T) {
	uc, repo := newUsecase(connFor(otherOrg))

	err := uc.Delete(authed(callerOrg), repository.DeleteTypeHard)
	require.Error(t, err)
	// The delete is HARD and takes the sealed credential with it, so not reaching the repository at
	// all is the assertion that matters.
	assert.False(t, repo.deleteCalled, "another workspace's connection must not be deleted")
}

func TestConnectionUsecase_DeleteScopesTheDeletionItself(t *testing.T) {
	uc, repo := newUsecase(connFor(callerOrg))

	require.NoError(t, uc.Delete(authed(callerOrg), repository.DeleteTypeHard))
	require.True(t, repo.deleteCalled)
	assert.Equal(t, callerOrg, ownerFilter(repo.deleteOpts),
		"the DELETE itself must carry the owner filter, not just the check before it")
}

// --- status ---

func TestConnectionUsecase_SetStatusRejectsAnUnknownStatus(t *testing.T) {
	uc, repo := newUsecase(connFor(callerOrg))

	_, err := uc.SetStatus(authed(callerOrg), connID, domain.ConnectionStatus("PROBABLY_FINE"))
	require.Error(t, err)
	assert.Nil(t, repo.patchOpts, "an invalid status must not reach the repository")
}

func TestConnectionUsecase_SetStatusRefusesAnotherWorkspacesConnection(t *testing.T) {
	uc, repo := newUsecase(connFor(otherOrg))

	_, err := uc.SetStatus(authed(callerOrg), connID, domain.ConnectionStatusRevoked)
	require.Error(t, err)
	assert.Nil(t, repo.patchOpts)
}

func TestConnectionUsecase_SetStatusRevokesWithoutDeleting(t *testing.T) {
	uc, repo := newUsecase(connFor(callerOrg))

	_, err := uc.SetStatus(authed(callerOrg), connID, domain.ConnectionStatusRevoked)
	require.NoError(t, err)

	require.NotNil(t, repo.patchOpts, "the status change must reach the repository")
	// The point of the endpoint: disabling leaves the credential sealed and in place, so re-enabling
	// does not mean pasting keys again.
	assert.False(t, repo.deleteCalled, "disabling must not delete anything")
}
