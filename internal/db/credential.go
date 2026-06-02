package db

import (
	"context"
	"errors"
	"time"

	apierrors "github.com/fromforgesoftware/go-kit/errors"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb"
	"github.com/fromforgesoftware/go-kit/persistence/postgres"
	"github.com/fromforgesoftware/go-kit/resource"
	"github.com/fromforgesoftware/go-kit/search"
	"github.com/fromforgesoftware/go-kit/slicesx"
	"gorm.io/gorm"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

var credentialFieldMapping = map[string]string{
	fields.ID:           "id",
	fields.CreatedAt:    "created_at",
	fields.UpdatedAt:    "updated_at",
	fields.ConnectionID: "connection_id",
	fields.Kind:         "kind",
	fields.KeyID:        "key_id",
	fields.ExpiresAt:    "expires_at",
}

type credentialEntity struct {
	EID           string     `gorm:"column:id;type:uuid;default:uuid_generate_v4();primaryKey"`
	ECreatedAt    time.Time  `gorm:"column:created_at;type:timestamptz;default:now()"`
	EUpdatedAt    time.Time  `gorm:"column:updated_at;type:timestamptz;default:now()"`
	EConnectionID string     `gorm:"column:connection_id;type:uuid"`
	EKind         string     `gorm:"column:kind"`
	ECiphertext   []byte     `gorm:"column:ciphertext;type:bytea"`
	EWrappedKey   []byte     `gorm:"column:wrapped_key;type:bytea"`
	EKeyID        string     `gorm:"column:key_id"`
	EExpiresAt    *time.Time `gorm:"column:expires_at"`
}

func (e *credentialEntity) TableName() string     { return "gleipnir.credential" }
func (e *credentialEntity) Type() resource.Type   { return domain.ResourceTypeCredential }
func (e *credentialEntity) ID() string            { return e.EID }
func (e *credentialEntity) LID() string           { return "" }
func (e *credentialEntity) CreatedAt() time.Time  { return e.ECreatedAt }
func (e *credentialEntity) UpdatedAt() time.Time  { return e.EUpdatedAt }
func (e *credentialEntity) DeletedAt() *time.Time { return nil }

func (e *credentialEntity) Kind() domain.CredentialKind { return domain.CredentialKind(e.EKind) }
func (e *credentialEntity) Ciphertext() []byte          { return e.ECiphertext }
func (e *credentialEntity) WrappedKey() []byte          { return e.EWrappedKey }
func (e *credentialEntity) KeyID() string               { return e.EKeyID }
func (e *credentialEntity) ExpiresAt() *time.Time       { return e.EExpiresAt }

func (e *credentialEntity) Connection() resource.Identifier {
	return resource.NewIdentifier(e.EConnectionID, domain.ResourceTypeConnection)
}

func credentialToEntity(c domain.Credential) *credentialEntity {
	return &credentialEntity{
		EID:           c.ID(),
		EConnectionID: idOf(c.Connection()),
		EKind:         string(c.Kind()),
		ECiphertext:   c.Ciphertext(),
		EWrappedKey:   c.WrappedKey(),
		EKeyID:        c.KeyID(),
		EExpiresAt:    c.ExpiresAt(),
	}
}

type credentialRepo struct {
	*postgres.Repo
}

func NewCredentialRepository(db *gormdb.DBClient) (*credentialRepo, error) {
	r, err := postgres.NewRepo(db, credentialFieldMapping)
	if err != nil {
		return nil, err
	}
	return &credentialRepo{Repo: r}, nil
}

func (r *credentialRepo) Create(ctx context.Context, c domain.Credential) (domain.Credential, error) {
	e := credentialToEntity(c)
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

func (r *credentialRepo) Get(ctx context.Context, opts ...search.Option) (domain.Credential, error) {
	s := search.New(opts...)
	var e credentialEntity
	if err := r.QueryApply(ctx, s.Query()).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.NotFound("credential", "")
		}
		return nil, postgres.NewErrUnknown(err)
	}
	return &e, nil
}

// Replace overwrites a credential's sealed material after a refresh, in place,
// so the connection's credential id stays stable for readers.
func (r *credentialRepo) Replace(ctx context.Context, id string, ciphertext, wrappedKey []byte, keyID string, expiresAt *time.Time) error {
	res := r.DB.WithContext(ctx).Exec(
		`UPDATE gleipnir.credential
		    SET ciphertext = ?, wrapped_key = ?, key_id = ?, expires_at = ?, updated_at = now()
		  WHERE id = ?`,
		ciphertext, wrappedKey, keyID, expiresAt, id,
	)
	if res.Error != nil {
		return postgres.NewErrUnknown(res.Error)
	}
	if res.RowsAffected == 0 {
		return apierrors.NotFound("credential", id)
	}
	return nil
}

// ListDueForRefresh returns credentials expiring at or before `before`, oldest
// first, bounded by limit — the refresh-ahead cron's work queue.
func (r *credentialRepo) ListDueForRefresh(ctx context.Context, before time.Time, limit int) ([]domain.Credential, error) {
	var found []*credentialEntity
	err := r.DB.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", before).
		Order("expires_at").
		Limit(limit).
		Find(&found).Error
	if err != nil {
		return nil, postgres.NewErrUnknown(err)
	}
	return slicesx.Map(found, func(e *credentialEntity) domain.Credential { return e }), nil
}
