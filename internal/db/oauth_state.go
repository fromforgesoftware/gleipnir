package db

import (
	"context"
	"errors"
	"time"

	apierrors "github.com/fromforgesoftware/go-kit/errors"
	"github.com/fromforgesoftware/go-kit/persistence/gormdb"
	"github.com/fromforgesoftware/go-kit/persistence/postgres"
	"github.com/fromforgesoftware/go-kit/resource"
	"gorm.io/gorm"

	"github.com/fromforgesoftware/gleipnir/internal/domain"
	"github.com/fromforgesoftware/gleipnir/internal/fields"
)

var oauthStateFieldMapping = map[string]string{
	fields.ID:           "id",
	fields.CreatedAt:    "created_at",
	fields.UpdatedAt:    "updated_at",
	fields.State:        "state",
	fields.ConnectionID: "connection_id",
	fields.Connector:    "connector",
	fields.ExpiresAt:    "expires_at",
}

type oauthStateEntity struct {
	EID           string     `gorm:"column:id;type:uuid;default:uuid_generate_v4();primaryKey"`
	ECreatedAt    time.Time  `gorm:"column:created_at;type:timestamptz;default:now()"`
	EUpdatedAt    time.Time  `gorm:"column:updated_at;type:timestamptz;default:now()"`
	EState        string     `gorm:"column:state"`
	EConnectionID string     `gorm:"column:connection_id;type:uuid"`
	EConnector    string     `gorm:"column:connector"`
	ERedirectURI  string     `gorm:"column:redirect_uri"`
	ECodeVerifier string     `gorm:"column:code_verifier"`
	EExpiresAt    time.Time  `gorm:"column:expires_at"`
	EConsumedAt   *time.Time `gorm:"column:consumed_at"`
}

func (e *oauthStateEntity) TableName() string     { return "gleipnir.oauth_state" }
func (e *oauthStateEntity) Type() resource.Type   { return domain.ResourceTypeOAuthState }
func (e *oauthStateEntity) ID() string            { return e.EID }
func (e *oauthStateEntity) LID() string           { return "" }
func (e *oauthStateEntity) CreatedAt() time.Time  { return e.ECreatedAt }
func (e *oauthStateEntity) UpdatedAt() time.Time  { return e.EUpdatedAt }
func (e *oauthStateEntity) DeletedAt() *time.Time { return nil }

func (e *oauthStateEntity) State() string        { return e.EState }
func (e *oauthStateEntity) Connector() string    { return e.EConnector }
func (e *oauthStateEntity) RedirectURI() string  { return e.ERedirectURI }
func (e *oauthStateEntity) CodeVerifier() string { return e.ECodeVerifier }
func (e *oauthStateEntity) ExpiresAt() time.Time { return e.EExpiresAt }
func (e *oauthStateEntity) ConsumedAt() *time.Time {
	return e.EConsumedAt
}

func (e *oauthStateEntity) Connection() resource.Identifier {
	return resource.NewIdentifier(e.EConnectionID, domain.ResourceTypeConnection)
}

func oauthStateToEntity(s domain.OAuthState) *oauthStateEntity {
	return &oauthStateEntity{
		EID:           s.ID(),
		EState:        s.State(),
		EConnectionID: idOf(s.Connection()),
		EConnector:    s.Connector(),
		ERedirectURI:  s.RedirectURI(),
		ECodeVerifier: s.CodeVerifier(),
		EExpiresAt:    s.ExpiresAt(),
		EConsumedAt:   s.ConsumedAt(),
	}
}

type oauthStateRepo struct {
	*postgres.Repo
}

func NewOAuthStateRepository(db *gormdb.DBClient) (*oauthStateRepo, error) {
	r, err := postgres.NewRepo(db, oauthStateFieldMapping)
	if err != nil {
		return nil, err
	}
	return &oauthStateRepo{Repo: r}, nil
}

func (r *oauthStateRepo) Create(ctx context.Context, s domain.OAuthState) (domain.OAuthState, error) {
	e := oauthStateToEntity(s)
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

// Consume atomically claims a usable state by its token: it must exist, not be
// expired, and not already consumed. The single UPDATE ... WHERE consumed_at IS
// NULL ... RETURNING guarantees at most one caller wins, so a replayed callback
// (or two concurrent callbacks) can succeed at most once. A miss returns
// NotFound — the caller maps that to a rejected callback without disclosing
// whether the token was unknown, expired, or already spent.
func (r *oauthStateRepo) Consume(ctx context.Context, state string, now time.Time) (domain.OAuthState, error) {
	var e oauthStateEntity
	res := r.DB.WithContext(ctx).Raw(
		`UPDATE gleipnir.oauth_state
		    SET consumed_at = ?, updated_at = now()
		  WHERE state = ?
		    AND consumed_at IS NULL
		    AND expires_at > ?
		  RETURNING *`,
		now, state, now,
	).Scan(&e)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, apierrors.NotFound("oauthState", "")
		}
		return nil, postgres.NewErrUnknown(res.Error)
	}
	// No row matched the (unconsumed AND unexpired) predicate: unknown, expired,
	// or already-consumed. RETURNING yields no row, so the scanned id is empty.
	if e.EID == "" {
		return nil, apierrors.NotFound("oauthState", "")
	}
	return &e, nil
}

// DeleteExpired removes states past their expiry (and so beyond replay-window
// usefulness), bounding table growth. Returns the number removed.
func (r *oauthStateRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB.WithContext(ctx).Exec(
		`DELETE FROM gleipnir.oauth_state WHERE expires_at <= ?`, before,
	)
	if res.Error != nil {
		return 0, postgres.NewErrUnknown(res.Error)
	}
	return res.RowsAffected, nil
}
