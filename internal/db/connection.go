// Package db holds Gleipnir's Postgres repositories.
package db

import (
	"context"
	"errors"
	"time"

	"github.com/fromforgesoftware/go-kit/application/repository"
	apierrors "github.com/fromforgesoftware/go-kit/errors"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb"
	"github.com/fromforgesoftware/go-kit/persistence/postgres"
	"github.com/fromforgesoftware/go-kit/resource"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/search/query"
	"github.com/fromforgesoftware/go-kit/slicesx"
	"gorm.io/gorm"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

var connectionFieldMapping = map[string]string{
	fields.ID:        "id",
	fields.CreatedAt: "created_at",
	fields.UpdatedAt: "updated_at",
	fields.Owner:     "owner",
	fields.Connector: "connector",
	fields.Status:    "status",
	fields.ExpiresAt: "expires_at",
}

type connectionEntity struct {
	EID        string     `gorm:"column:id;type:uuid;default:uuid_generate_v4();primaryKey"`
	ECreatedAt time.Time  `gorm:"column:created_at;type:timestamptz;default:now()"`
	EUpdatedAt time.Time  `gorm:"column:updated_at;type:timestamptz;default:now()"`
	EOwner     string     `gorm:"column:owner"`
	EConnector string     `gorm:"column:connector"`
	EStatus    string     `gorm:"column:status"`
	EScopes    []string   `gorm:"column:scopes;type:jsonb;serializer:json"`
	EExpiresAt *time.Time `gorm:"column:expires_at"`
}

func (e *connectionEntity) TableName() string     { return "gleipnir.connection" }
func (e *connectionEntity) Type() resource.Type   { return domain.ResourceTypeConnection }
func (e *connectionEntity) ID() string            { return e.EID }
func (e *connectionEntity) LID() string           { return "" }
func (e *connectionEntity) CreatedAt() time.Time  { return e.ECreatedAt }
func (e *connectionEntity) UpdatedAt() time.Time  { return e.EUpdatedAt }
func (e *connectionEntity) DeletedAt() *time.Time { return nil }

func (e *connectionEntity) Owner() string     { return e.EOwner }
func (e *connectionEntity) Connector() string { return e.EConnector }
func (e *connectionEntity) Status() domain.ConnectionStatus {
	return domain.ConnectionStatus(e.EStatus)
}
func (e *connectionEntity) Scopes() []string      { return e.EScopes }
func (e *connectionEntity) ExpiresAt() *time.Time { return e.EExpiresAt }

func connectionToEntity(c domain.Connection) *connectionEntity {
	scopes := c.Scopes()
	if scopes == nil {
		scopes = []string{}
	}
	return &connectionEntity{
		EID:        c.ID(),
		EOwner:     c.Owner(),
		EConnector: c.Connector(),
		EStatus:    string(c.Status()),
		EScopes:    scopes,
		EExpiresAt: c.ExpiresAt(),
	}
}

func idOf(i resource.Identifier) string {
	if i == nil {
		return ""
	}
	return i.ID()
}

type connectionRepo struct {
	*postgres.Repo
}

func NewConnectionRepository(db *gormdb.DBClient) (*connectionRepo, error) {
	r, err := postgres.NewRepo(db, connectionFieldMapping)
	if err != nil {
		return nil, err
	}
	return &connectionRepo{Repo: r}, nil
}

func (r *connectionRepo) Create(ctx context.Context, c domain.Connection) (domain.Connection, error) {
	e := connectionToEntity(c)
	tx := r.DB.WithContext(ctx)
	if e.EID == "" {
		tx = tx.Omit("id")
	}
	if e.ECreatedAt.IsZero() {
		tx = tx.Omit("created_at", "updated_at")
	}
	if err := tx.Create(e).Error; err != nil {
		return nil, postgres.NewErrUnknown(err)
	}
	return e, nil
}

func (r *connectionRepo) Get(ctx context.Context, opts ...search.Option) (domain.Connection, error) {
	s := search.New(opts...)
	var e connectionEntity
	if err := r.QueryApply(ctx, s.Query()).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.NotFound("connection", "")
		}
		return nil, postgres.NewErrUnknown(err)
	}
	return &e, nil
}

func (r *connectionRepo) List(ctx context.Context, opts ...search.Option) (resource.ListResponse[domain.Connection], error) {
	q := search.New(opts...).Query()
	var found []*connectionEntity
	if err := r.QueryApply(ctx, q).Find(&found).Error; err != nil {
		return nil, postgres.NewErrUnknown(err)
	}
	var total int64
	if err := r.CountApply(ctx, new(connectionEntity), q).Count(&total).Error; err != nil {
		return nil, postgres.NewErrUnknown(err)
	}
	out := slicesx.Map(found, func(e *connectionEntity) domain.Connection { return e })
	return resource.NewListResponse(out, int(total)), nil
}

func (r *connectionRepo) SetStatus(ctx context.Context, id string, status domain.ConnectionStatus) error {
	res := r.DB.WithContext(ctx).Exec(
		`UPDATE gleipnir.connection SET status = ?, updated_at = now() WHERE id = ?`,
		string(status), id,
	)
	if res.Error != nil {
		return postgres.NewErrUnknown(res.Error)
	}
	if res.RowsAffected == 0 {
		return apierrors.NotFound("connection", id)
	}
	return nil
}

func (r *connectionRepo) Delete(ctx context.Context, delType repository.DeleteType, opts ...search.Option) error {
	q := search.New(opts...).Query()
	if err := query.Validate(q, query.MandatoryFilters(fields.ID)); err != nil {
		return err
	}
	tx := r.QueryApply(ctx, q).Model(&connectionEntity{})
	if delType == repository.DeleteTypeHard {
		tx = tx.Unscoped()
	}
	if err := tx.Delete(&connectionEntity{}).Error; err != nil {
		return postgres.NewErrUnknown(err)
	}
	return nil
}
