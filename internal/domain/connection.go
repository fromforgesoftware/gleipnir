package domain

import (
	"time"

	"github.com/fromforgesoftware/go-kit/resource"
)

// ResourceTypeConnection is the JSON:API type for /api/connections.
const ResourceTypeConnection resource.Type = "connections"

// ConnectionStatus tracks an authorized connection's lifecycle.
type ConnectionStatus string

const (
	ConnectionStatusActive      ConnectionStatus = "ACTIVE"
	ConnectionStatusNeedsReauth ConnectionStatus = "NEEDS_REAUTH"
	ConnectionStatusRevoked     ConnectionStatus = "REVOKED"
	ConnectionStatusError       ConnectionStatus = "ERROR"
)

func (s ConnectionStatus) Valid() bool {
	switch s {
	case ConnectionStatusActive, ConnectionStatusNeedsReauth, ConnectionStatusRevoked, ConnectionStatusError:
		return true
	}
	return false
}

// Connection is an owner's authorized instance of a connector. Owner is an
// opaque key the caller supplies (a user, org, workspace, or service id —
// Gleipnir never interprets it); every access is scoped by it. The secret
// material lives in a Credential.
type Connection interface {
	resource.Resource
	Owner() string
	Connector() string // connector slug
	Status() ConnectionStatus
	Scopes() []string
	ExpiresAt() *time.Time
}

type connection struct {
	resource.Resource

	owner     string
	connector string
	status    ConnectionStatus
	scopes    []string
	expiresAt *time.Time
}

type ConnectionOption func(*connection)

func WithConnectionID(id string) ConnectionOption {
	return func(c *connection) { c.Resource = resource.Update(c.Resource, resource.WithID(id)) }
}
func WithConnectionStatus(s ConnectionStatus) ConnectionOption {
	return func(c *connection) { c.status = s }
}
func WithConnectionScopes(s []string) ConnectionOption {
	return func(c *connection) { c.scopes = s }
}
func WithConnectionExpiresAt(t *time.Time) ConnectionOption {
	return func(c *connection) { c.expiresAt = t }
}

// NewConnection builds a connection aggregate. owner/connector are mandatory;
// status defaults to ACTIVE.
func NewConnection(owner, connector string, opts ...ConnectionOption) Connection {
	c := &connection{
		Resource:  resource.New(resource.WithType(ResourceTypeConnection)),
		owner:     owner,
		connector: connector,
		status:    ConnectionStatusActive,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *connection) Owner() string            { return c.owner }
func (c *connection) Connector() string        { return c.connector }
func (c *connection) Status() ConnectionStatus { return c.status }
func (c *connection) Scopes() []string         { return c.scopes }
func (c *connection) ExpiresAt() *time.Time    { return c.expiresAt }
