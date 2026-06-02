package app

import (
	"context"

	"github.com/fromforgesoftware/go-kit/application/repository"
	"github.com/fromforgesoftware/go-kit/application/usecase"
	apierrors "github.com/fromforgesoftware/go-kit/errors"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
)

// ConnectionRepository persists connections via kit generics.
type ConnectionRepository interface {
	repository.Creator[domain.Connection]
	repository.Getter[domain.Connection]
	repository.Lister[domain.Connection]
	repository.Deleter
}

// ConnectionUsecase is the management surface for connections. Reads and
// deletes flow through the kit generic handlers (List filtered by the caller's
// owner); Create validates the connector exists and requires an owner.
type ConnectionUsecase interface {
	repository.Getter[domain.Connection]
	repository.Lister[domain.Connection]
	repository.Deleter
	Create(ctx context.Context, conn domain.Connection) (domain.Connection, error)
}

type connectionUsecase struct {
	usecase.Getter[domain.Connection]
	usecase.Lister[domain.Connection]
	repository.Deleter

	conns    ConnectionRepository
	registry ConnectorRegistry
}

func NewConnectionUsecase(conns ConnectionRepository, registry ConnectorRegistry) ConnectionUsecase {
	return &connectionUsecase{
		Getter:   usecase.NewGetter(conns, domain.ResourceTypeConnection),
		Lister:   usecase.NewLister(conns),
		Deleter:  usecase.NewDeleter(conns),
		conns:    conns,
		registry: registry,
	}
}

func (u *connectionUsecase) Create(ctx context.Context, conn domain.Connection) (domain.Connection, error) {
	if conn.Owner() == "" {
		return nil, apierrors.InvalidArgument("owner is required")
	}
	if _, ok := u.registry.Lookup(conn.Connector()); !ok {
		return nil, apierrors.InvalidArgument("unknown connector: " + conn.Connector())
	}
	return u.conns.Create(ctx, conn)
}
