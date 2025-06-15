package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type ssConfig struct {
	Raw      string `json:"-"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Method   string `json:"method"`
	Password string `json:"password"`
	Remark   string `json:"remark"`
}

type ssJSONServer struct {
	Address  string `json:"address"`
	Method   string `json:"method"`
	OTA      bool   `json:"ota"`
	Password string `json:"password"`
	Port     int    `json:"port"`
}

type ssJSONSettings struct {
	Servers []ssJSONServer `json:"servers"`
}

func (c *ssConfig) MarshalJSON() ([]byte, error) {
	server := ssJSONServer{
		Address:  c.Server,
		Method:   c.Method,
		Password: c.Password,
		Port:     c.Port,
	}
	return json.Marshal(basicOutboundConfig{
		Protocol: "shadowsocks",
		Settings: ssJSONSettings{Servers: []ssJSONServer{server}},
	})
}

func parseSS(link string) (Config, error) {
	parts := strings.Split(link, "://")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	config := &ssConfig{Remark: parts[0]}
	data := strings.Split(parts[1], "@")
	if len(data) != 2 {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	authData, err := base64.RawStdEncoding.DecodeString(data[0])
	if err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	auth := strings.Split(string(authData), ":")
	if len(auth) != 2 {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}
	config.Method = auth[0]
	config.Password = auth[1]

	serverParts := strings.Split(data[1], "#")
	if len(serverParts) == 2 {
		config.Remark = serverParts[1]
	}

	host, port, err := net.SplitHostPort(serverParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	config.Server = host
	config.Port, err = strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	return config, nil
}
