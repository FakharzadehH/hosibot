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
		startOnFirstUse := p.Connection == "onconecton"
		return NewXUIClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, p.InboundID, p.LinkSubX, startOnFirstUse), nil
	case "hiddify":
		apiKey := p.SecretCode
		if apiKey == "" {
			apiKey = p.PasswordPanel
		}
		return NewHiddifyClient(p.URLPanel, apiKey, p.LinkSubX), nil
	case "marzneshin":
		defaultServices := parseStringJSONArray(p.Proxies)
		startOnFirstUse := p.Connection == "onconecton"
		return NewMarzneshinClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, defaultServices, startOnFirstUse), nil
	case "s_ui":
		return NewSUIClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel), nil
	case "WGDashboard", "wgdashboard":
		apiKey := p.PasswordPanel
		if apiKey == "" {
			apiKey = p.SecretCode
		}
		return NewWGDashboardClient(p.URLPanel, apiKey, p.InboundID), nil
	case "mikrotik":
		return NewMikrotikClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, p.Proxies), nil
	case "ibsng":
		return NewIBSngClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, p.Proxies), nil
	case "alireza_single":
		startOnFirstUse := p.Connection == "onconecton"
		return NewAlirezaSingleClient(p.URLPanel, p.UsernamePanel, p.PasswordPanel, p.InboundID, p.LinkSubX, startOnFirstUse), nil
	default:
		return nil, fmt.Errorf("unsupported panel type: %s", p.Type)
	}
}

func parseStringJSONArray(raw string) []string {
	if raw == "" {
		return nil
	}

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	// Some configs may store object maps; use keys as fallback.
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}

	return nil
}
