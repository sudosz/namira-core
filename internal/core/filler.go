package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/enescakir/emoji"
	"github.com/oschwald/geoip2-golang"
)

// Protocol emojis
var protocolEmojis = map[string]emoji.Emoji{
	"vmess":  emoji.HighVoltage,
	"vless":  emoji.Rocket,
	"trojan": emoji.Shield,
	"ss":     emoji.Locked,
}

var geoipDB *geoip2.Reader

func init() {
	var err error
	geoipDB, err = geoip2.Open("GeoLite2-Country.mmdb")
	if err != nil {
		fmt.Printf("Warning: Could not open GeoLite2 database: %v\n", err)
	}
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

func (c *Core) FillCheckResult(result *CheckResult, template ...RemarkTemplate) {
	tmpl := c.remarkTemplate
	if len(template) > 0 {
		tmpl = template[0]
	}

	parts := strings.SplitN(result.Raw, "://", 2)
	if len(parts) != 2 {
		return
	}

	result.Protocol = parts[0]
	switch result.Protocol {
	case "vmess":
		c.fillVMessResult(result, tmpl)
	case "vless", "trojan", "ss":
		c.fillURLResult(result, tmpl, result.Protocol)
	}
}

func (c *Core) fillVMessResult(result *CheckResult, tmpl RemarkTemplate) {
	parts := strings.SplitN(result.Raw, "://", 2)
	if len(parts) != 2 {
		return
	}

	// Decode VMess config
	data, err := base64Decode(parts[1])
	if err != nil {
		return
	}

	var vmessConfig map[string]interface{}
	if err := json.Unmarshal(data, &vmessConfig); err != nil {
		return
	}

	// Extract server info
	server, _ := vmessConfig["add"].(string)
	result.Server = server
	result.CountryCode = getCountryFromServer(server)
	result.Remark = c.generateRemark(server, "vmess", tmpl)
	vmessConfig["ps"] = result.Remark

	if newData, err := json.Marshal(vmessConfig); err == nil {
		result.Raw = parts[0] + "://" + base64Encode(newData)
	}
}

func (c *Core) fillURLResult(result *CheckResult, tmpl RemarkTemplate, protocol string) {
	result.Raw = strings.Split(result.Raw, "#")[0]
	server := extractServerFromURL(result.Raw)
	result.CountryCode = getCountryFromServer(server)
	result.Remark = c.generateRemark(server, protocol, tmpl)
	result.Server = server
	result.Raw += "#" + url.PathEscape(result.Remark)
}

func (c *Core) generateRemark(server, protocol string, tmpl RemarkTemplate) string {
	parts := []string{"✨ " + tmpl.OrgName}

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

	if tmpl.ShowCountry && server != "" {
		if countryCode := getCountryFromServer(server); countryCode != "" {
			if countryFlag, err := emoji.CountryFlag(countryCode); err == nil {
				parts = append(parts, countryFlag.String())
			} else {
				parts = append(parts, countryCode)
			}
		}
	}

	return strings.Join(parts, tmpl.Separator)
}

func getCountryFromServer(server string) string {
	if server == "" || geoipDB == nil {
		return ""
	}

	ip := server
	if !net.ParseIP(server).IsUnspecified() {
		if ips, err := net.LookupIP(server); err == nil && len(ips) > 0 {
			ip = ips[0].String()
		}
	}

	// Try to parse IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ""
	}

	// Lookup country
	record, err := geoipDB.Country(parsedIP)
	if err != nil {
		return ""
	}

	if code := record.Country.IsoCode; code != "" {
		return code
	}
	return record.RegisteredCountry.IsoCode
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
