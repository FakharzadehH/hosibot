package panel

import "context"

// PanelUser represents a user on a VPN panel.
type PanelUser struct {
	Username       string            `json:"username"`
	Status         string            `json:"status"` // active, disabled, limited, expired
	DataLimit      int64             `json:"data_limit"`
	UsedTraffic    int64             `json:"used_traffic"`
	ExpireTime     int64             `json:"expire_time"`
	SubLink        string            `json:"sub_link"`
	OnlineAt       int64             `json:"online_at"`
	OnlineStatus   string            `json:"online_status,omitempty"` // online/offline
	SubUpdatedAt   string            `json:"sub_updated_at,omitempty"`
	SubLastAgent   string            `json:"sub_last_user_agent,omitempty"`
	Links          []string          `json:"links"`
	Inbounds       map[string]string `json:"inbounds,omitempty"`
	Proxies        map[string]string `json:"proxies,omitempty"`
	Note           string            `json:"note,omitempty"`
	DataLimitReset string            `json:"data_limit_reset_strategy,omitempty"`
}

// PanelInbound represents an inbound/protocol on a panel.
type PanelInbound struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

// CreateUserRequest contains params for creating a user on a panel.
type CreateUserRequest struct {
	Username       string              `json:"username"`
	DataLimit      int64               `json:"data_limit"` // bytes
	ExpireDays     int                 `json:"expire_days"`
	Inbounds       map[string][]string `json:"inbounds,omitempty"`
	Proxies        map[string]string   `json:"proxies,omitempty"`
	Note           string              `json:"note,omitempty"`
	DataLimitReset string              `json:"data_limit_reset_strategy,omitempty"`
	FlowType       string              `json:"flow,omitempty"`
}

// ModifyUserRequest contains params for modifying a user on a panel.
type ModifyUserRequest struct {
	Status         string              `json:"status,omitempty"`
	DataLimit      int64               `json:"data_limit,omitempty"`
	ExpireTime     int64               `json:"expire_time,omitempty"`
	Inbounds       map[string][]string `json:"inbounds,omitempty"`
	Note           string              `json:"note,omitempty"`
	DataLimitReset string              `json:"data_limit_reset_strategy,omitempty"`
}

// PanelClient defines the interface for VPN panel integrations.
// Each panel type (Marzban, X-UI, Hiddify, etc.) implements this interface.
type PanelClient interface {
	// Authenticate logs in and stores the auth token/session.
	Authenticate(ctx context.Context) error

	// GetUser gets a user by username.
	GetUser(ctx context.Context, username string) (*PanelUser, error)

	// CreateUser creates a new user on the panel.
	CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error)

	// ModifyUser modifies an existing user.
	ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error)

	// DeleteUser removes a user from the panel.
	DeleteUser(ctx context.Context, username string) error

	// EnableUser enables a user account.
	EnableUser(ctx context.Context, username string) error

	// DisableUser disables a user account.
	DisableUser(ctx context.Context, username string) error

	// ResetTraffic resets traffic usage for a user.
	ResetTraffic(ctx context.Context, username string) error

	// RevokeSubscription revokes and regenerates the subscription link.
	RevokeSubscription(ctx context.Context, username string) (string, error)

	// GetInbounds returns available inbounds/protocols on the panel.
	GetInbounds(ctx context.Context) ([]PanelInbound, error)

	// GetSystemStats returns panel system statistics (online users, traffic, etc.).
	GetSystemStats(ctx context.Context) (map[string]interface{}, error)

	// GetSubscriptionLink returns the subscription link for a user.
	GetSubscriptionLink(ctx context.Context, username string) (string, error)

	// PanelType returns the panel type identifier.
	PanelType() string
}
