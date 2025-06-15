package parser

import (
	"fmt"
	"strings"
)

type Config interface {
	MarshalJSON() ([]byte, error)
}

var (
	ErrInvalidConfig   = fmt.Errorf("invalid config")
	ErrUnsupportedType = fmt.Errorf("unsupported config type")
	ErrInvalidFormat   = fmt.Errorf("invalid config format")
)

type basicOutboundConfig struct {
	Protocol string `json:"protocol"`
	Settings any    `json:"settings"`
}

type ConfigParser func(string) (Config, error)

type Parser struct {
	parsers map[string]ConfigParser
}

// TODO: Implement the parsers for VLESS, VMess, Trojan, and other protocols.
func NewParser() *Parser {
	return &Parser{
		parsers: map[string]ConfigParser{
			"ss": parseSS,
			// "vless":  parseVless,
			// "vmess":  parseVmess,
			// "trojan": parseTrojan,
		},
	}
}

func (p *Parser) Parse(config string) (Config, error) {
	parts := strings.SplitN(config, "://", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	parser, exists := p.parsers[parts[0]]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, config)
	}

	return parser(config)
}
