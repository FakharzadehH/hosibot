package panel

import (
	"context"
	"fmt"
)

// unsupportedPanelClient is a placeholder adapter for legacy panel types that
// do not yet have full native implementations in Go.
type unsupportedPanelClient struct {
	panelType string
}

func newUnsupportedPanelClient(panelType string) PanelClient {
	return &unsupportedPanelClient{panelType: panelType}
}

func (u *unsupportedPanelClient) err() error {
	return fmt.Errorf("panel type %s is recognized but not implemented yet", u.panelType)
}

func (u *unsupportedPanelClient) Authenticate(ctx context.Context) error { return u.err() }
func (u *unsupportedPanelClient) GetUser(ctx context.Context, username string) (*PanelUser, error) {
	return nil, u.err()
}
func (u *unsupportedPanelClient) CreateUser(ctx context.Context, req CreateUserRequest) (*PanelUser, error) {
	return nil, u.err()
}
func (u *unsupportedPanelClient) ModifyUser(ctx context.Context, username string, req ModifyUserRequest) (*PanelUser, error) {
	return nil, u.err()
}
func (u *unsupportedPanelClient) DeleteUser(ctx context.Context, username string) error {
	return u.err()
}
func (u *unsupportedPanelClient) EnableUser(ctx context.Context, username string) error {
	return u.err()
}
func (u *unsupportedPanelClient) DisableUser(ctx context.Context, username string) error {
	return u.err()
}
func (u *unsupportedPanelClient) ResetTraffic(ctx context.Context, username string) error {
	return u.err()
}
func (u *unsupportedPanelClient) RevokeSubscription(ctx context.Context, username string) (string, error) {
	return "", u.err()
}
func (u *unsupportedPanelClient) GetInbounds(ctx context.Context) ([]PanelInbound, error) {
	return nil, u.err()
}
func (u *unsupportedPanelClient) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, u.err()
}
func (u *unsupportedPanelClient) GetSubscriptionLink(ctx context.Context, username string) (string, error) {
	return "", u.err()
}
func (u *unsupportedPanelClient) PanelType() string { return u.panelType }

func NewXUIClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("x-ui_single")
}

func NewHiddifyClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("hiddify")
}

func NewMarzneshinClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("marzneshin")
}

func NewWGDashboardClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("wgdashboard")
}

func NewSUIClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("s_ui")
}

func NewMikrotikClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("mikrotik")
}

func NewIBSngClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("ibsng")
}

func NewAlirezaSingleClient(baseURL, username, password string) PanelClient {
	return newUnsupportedPanelClient("alireza_single")
}
