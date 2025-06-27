package qr

import (
	"fmt"
	"net/url"
)

const (
	qrAPIURL      = "https://api.qrcode-monkey.com/qr/custom"
	defaultSize   = "1024"
	defaultFormat = "png"
)

type QRGenerator struct {
	config string
}

func NewQRGenerator(config string) *QRGenerator {
	return &QRGenerator{config: config}
}

func (q *QRGenerator) GenerateURL(data string) string {
	params := make(url.Values)
	params.Set("download", "true")
	params.Set("file", defaultFormat)
	params.Set("size", defaultSize)
	params.Set("config", q.config)
	params.Set("data", data)

	return fmt.Sprintf("%s?%s", qrAPIURL, params.Encode())
}
