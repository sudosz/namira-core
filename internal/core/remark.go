package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/enescakir/emoji"
)

// Protocol emojis
var protocolEmojis = map[string]emoji.Emoji{
	"vmess":  emoji.HighVoltage,
	"vless":  emoji.Rocket,
	"trojan": emoji.Shield,
	"ss":     emoji.Locked,
}

type CountryResponse struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
}

type RemarkTemplate struct {
	OrgName      string
	Separator    string
	ShowCountry  bool
	ShowHost     bool
	ShowProtocol bool
}

func DefaultRemarkTemplate() RemarkTemplate {
	return RemarkTemplate{
		OrgName:      "NaMiraNet",
		Separator:    " | ",
		ShowCountry:  true,
		ShowHost:     true,
		ShowProtocol: true,
	}
}

func (c *Core) ReplaceConfigRemark(config string, template ...RemarkTemplate) string {
	tmpl := DefaultRemarkTemplate()
	if len(template) > 0 {
		tmpl = template[0]
	}

	protocol := strings.SplitN(config, "://", 2)[0]
	if protocol == "" {
		return config
	}

	switch protocol {
	case "vmess":
		return c.replaceVMessRemark(config, tmpl)
	case "vless", "trojan", "ss":
		return c.replaceURLRemark(config, tmpl, protocol)
	default:
		return config
	}
}

func (c *Core) replaceVMessRemark(config string, tmpl RemarkTemplate) string {
	parts := strings.SplitN(config, "://", 2)
	if len(parts) != 2 {
		return config
	}

	// Decode VMess config
	data, err := base64Decode(parts[1])
	if err != nil {
		return config
	}

	var vmessConfig map[string]interface{}
	if err := json.Unmarshal(data, &vmessConfig); err != nil {
		return config
	}

	// Extract server info
	server, _ := vmessConfig["add"].(string)
	vmessConfig["ps"] = c.generateRemark(server, "vmess", tmpl)

	newData, err := json.Marshal(vmessConfig)
	if err != nil {
		return config
	}

	return parts[0] + "://" + base64Encode(newData)
}

func (c *Core) replaceURLRemark(config string, tmpl RemarkTemplate, protocol string) string {
	// Find existing remark (after #)

	if hashIndex := strings.LastIndex(config, "#"); hashIndex != -1 {
		config = config[:hashIndex]
	}

	// Extract server from URL
	server := extractServerFromURL(config)
	remark := c.generateRemark(server, protocol, tmpl)

	return config + "#" + url.PathEscape(remark)
}

func (c *Core) generateRemark(server, protocol string, tmpl RemarkTemplate) string {
	var parts []string

	// Add organization with sparkles
	parts = append(parts, "✨ "+tmpl.OrgName)

	// Add protocol emoji
	if tmpl.ShowProtocol {
		if protocolEmoji, exists := protocolEmojis[protocol]; exists {
			parts = append(parts, protocolEmoji.String())
		}
	}

	if tmpl.ShowHost && server != "" {
		if host := extractHost(server); host != "" {
			parts = append(parts, "🌐 "+host)
		}
	}

	if tmpl.ShowCountry {
		if countryCode := getCountryFromServer(server); countryCode != "" {
			countryFlag, err := emoji.CountryFlag(strings.ToLower(countryCode))
			if err == nil {
				parts = append(parts, countryFlag.String())
			} else {
				parts = append(parts, "🏁 "+countryCode)
			}
		}
	}

	return strings.Join(parts, tmpl.Separator)
}

func getCountryFromServer(server string) string {
	if server == "" {
		return ""
	}

	ip := server
	if !net.ParseIP(server).IsUnspecified() {
		if ips, err := net.LookupIP(server); err == nil && len(ips) > 0 {
			ip = ips[0].String()
		}
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(fmt.Sprintf("https://api.country.is/%s", ip))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var countryResp CountryResponse
	if err := json.NewDecoder(resp.Body).Decode(&countryResp); err != nil {
		return ""
	}

	return countryResp.Country
}

func extractServerFromURL(config string) string {
	// Remove protocol
	parts := strings.SplitN(config, "://", 2)
	if len(parts) != 2 {
		return ""
	}

	// Remove user info and extract host
	urlPart := parts[1]
	if idx := strings.Index(urlPart, "@"); idx != -1 {
		urlPart = urlPart[idx+1:]
	}

	// Remove path and query
	if idx := strings.IndexAny(urlPart, "/?#"); idx != -1 {
		urlPart = urlPart[:idx]
	}

	// Extract host from host:port
	host, _, _ := net.SplitHostPort(urlPart)
	if host == "" {
		host = urlPart
	}

	return host
}

func extractHost(server string) string {
	if server == "" {
		return ""
	}

	// Remove port if present
	if host, _, err := net.SplitHostPort(server); err == nil {
		return host
	}
	return server
}

func base64Decode(data string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(data)
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
