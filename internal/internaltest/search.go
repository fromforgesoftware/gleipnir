//go:build integration

package internaltest

import (
	"github.com/fromforgesoftware/go-kit/filter"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/search/query"

	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

func GetByID(id string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.ID, id))
}

func GetByConnection(connectionID string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.ConnectionID, connectionID))
}

func FilterByOwner(owner string) search.Option {
	return search.WithQueryOpts(query.FilterBy(filter.OpEq, fields.Owner, owner))
}
