package panel

import (
	"encoding/json"
	"fmt"

	"hosibot/internal/models"
)

// PanelFactory creates a PanelClient based on the panel type.
func PanelFactory(p *models.Panel) (PanelClient, error) {
	switch p.Type {
	case "marzban":
		return NewMarzbanClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "pasarguard":
		client := NewPasarGuardClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, p.SecretCode)
		// Set default inbounds from panel config if available
		if p.Inbounds != "" {
			var inboundMap map[string][]string
			if err := json.Unmarshal([]byte(p.Inbounds), &inboundMap); err == nil {
				var protocols []string
				for protocol := range inboundMap {
					protocols = append(protocols, protocol)
				}
				client.SetInbounds(protocols)
			} else {
				// Try as flat string array
				var tags []string
				if err := json.Unmarshal([]byte(p.Inbounds), &tags); err == nil {
					client.SetInbounds(tags)
				}
			}
		}
		return client, nil
	case "x-ui_single", "xui":
		return NewXUIClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "hiddify":
		return NewHiddifyClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "marzneshin":
		return NewMarzneshinClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "s_ui":
		return NewSUIClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "WGDashboard", "wgdashboard":
		return NewWGDashboardClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "mikrotik":
		return NewMikrotikClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "ibsng":
		return NewIBSngClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "alireza_single":
		return NewAlirezaSingleClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	default:
		return nil, fmt.Errorf("unsupported panel type: %s", p.Type)
	}
}
