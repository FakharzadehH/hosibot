package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hosibot/internal/models"
	"hosibot/internal/panel"
)

func extractPanelTemplate(panelModel *models.Panel, username string) (string, string, error) {
	client, err := panel.PanelFactory(panelModel)
	if err != nil {
		return "", "", err
	}

	ctx := context.Background()
	if err := client.Authenticate(ctx); err != nil {
		return "", "", err
	}

	switch p := client.(type) {
	case *panel.MarzbanClient:
		inbounds, proxies, err := p.GetUserTemplate(ctx, username)
		if err != nil {
			return "", "", err
		}
		inboundsJSON, _ := json.Marshal(inbounds)
		proxiesJSON, _ := json.Marshal(proxies)
		return string(inboundsJSON), string(proxiesJSON), nil
	case *panel.PasarGuardClient:
		user, err := p.GetUser(ctx, username)
		if err != nil {
			return "", "", err
		}
		inbounds := make(map[string][]string)
		for proto := range user.Proxies {
			inbounds[strings.TrimSpace(proto)] = []string{}
		}
		if len(inbounds) == 0 {
			return "", "", fmt.Errorf("user template not found")
		}
		inboundsJSON, _ := json.Marshal(inbounds)
		proxiesJSON, _ := json.Marshal(user.Proxies)
		return string(inboundsJSON), string(proxiesJSON), nil
	default:
		return "", "", fmt.Errorf("panel_not_support_options")
	}
}
